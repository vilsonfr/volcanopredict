# Anatomia de um adaptador de fonte

Este documento é para quem vai escrever o **segundo** conector. Ele descreve
a forma que o primeiro tem, e por que ela é assim, para que ninguém precise
ler `internal/usgs` inteiro antes de começar.

O primeiro conector é o do USGS. Ele não é uma abstração genérica de fonte,
de propósito: a forma comum se extrai do segundo conector, não se adivinha
no primeiro (design.md, Non-Goals). O que segue é o molde, não um framework.

## A cadeia

A §9 da master spec exige que toda integração passe por etapas separadas, e
que uma falha diga **em qual etapa** ocorreu:

```
fetch  →  parse  →  validate  →  normalize  →  persist
```

Elas são separadas porque falham de formas diferentes e a resposta a cada
falha é diferente:

| Etapa | O que pode dar errado | O que se faz |
|---|---|---|
| fetch | fonte fora do ar, timeout, corpo truncado | tenta de novo com espera crescente; se persistir, a execução falha inteira |
| parse | o documento não tem a forma esperada | **aborta a resposta toda** — importar parte de um documento incompreensível é importar um palpite |
| validate | um registro específico é impossível | rejeita **só aquele**, conta, e segue com os demais |
| normalize | — | unidade explícita, UTC, SRID 4326, identificador da fonte preservado |
| persist | conflito, indisponibilidade do banco | a execução falha e fica registrada |

A distinção que mais importa é entre as duas do meio: **documento
malformado aborta; registro inválido não.** Um documento que não é o que se
esperava significa que não se sabe o que se está lendo. Um registro ruim
dentro de um documento válido é só um registro ruim.

## As oito coisas que um adaptador precisa fazer

### 1. Resolver a fonte antes de qualquer coisa

Nenhum dado externo entra a partir de fonte não registrada ou desabilitada:

```go
src, err := source.GetEnabledByName(ctx, pool, MinhaFonte)
```

É isto que impede uma fonte bloqueada — como o PVMBG — de ser coletada por
acidente. O adaptador **nunca** cria a linha em `data_sources`; ela vem de
migração, depois de alguém ler os termos.

### 2. Identificar-se e se limitar na rede

Timeout por requisição, número máximo de tentativas, espera crescente entre
elas, e `User-Agent` que diz quem está chamando. Uma plataforma científica
que sobrecarrega anonimamente a fonte de que depende não merece o dado.

Não repita um `4xx`. Uma consulta malformada não melhora sendo repetida; a
repetição só adiciona carga.

### 3. Distinguir ausente de zero

Campos que a fonte pode omitir são **ponteiros**, do topo ao fundo. Um
`float64` simples lê `"mag": 0` e `"mag": null` de forma idêntica.

Isso não é hipotético: `ci41531128` é um sismo real com magnitude
**exatamente 0.0** que omite `nst`, `dmin` e `gap`. Está versionado em
`backend/testdata/usgs/query_absent_vs_zero.json` justamente por isso.

### 4. Guardar o dado cru e a versão do parser

Na mesma linha, na mesma transação:

- **`raw`** — o payload como a fonte mandou, para que qualquer valor
  derivado seja reconstruível sem consultar a fonte de novo (§18).
- **`parser_version`** — uma constante no pacote, incrementada sempre que a
  extração mudar o que põe num campo. Sem ela, um bug de parse corrigido
  depois torna impossível saber quais linhas nasceram erradas.

### 5. Avaliar qualidade na escrita, nunca na leitura

```go
verdict := dataquality.Evaluate(obs, opts, seenKeys)
```

O veredito é atributo **da versão**, decidido com as regras vigentes no
momento em que ela foi escrita, e gravado. Calcular na leitura faria o
passado mudar toda vez que uma regra mudasse — exatamente o que o modelo
bitemporal existe para impedir.

`dataquality.Verdict` não tem zero-value útil: esquecer de avaliar é erro, e
`earthquake.Save` recusa. Registro que falha verificação é **marcado e
guardado**, nunca descartado.

Uma armadilha medida na prática: **atraso é propriedade da coleta, não do
evento.** A regra de chegada tardia só faz sentido em ciclo incremental. Na
primeira ingestão real de backfill, ela marcou os 642 registros como
`delayed`, porque todo registro histórico é trivialmente "atrasado" — o
campo deixou de significar coisa alguma. Veja `qualityOptionsFor`.

### 6. Deduplicar por conteúdo, não pelo `updated` da fonte

O USGS reemite payloads com `updated` novo sem ter mudado nada. Confiar
nesse campo criaria uma versão por ciclo, para sempre.

Compare o **conteúdo normalizado** com a versão corrente. Se for igual, não
escreva nada. É isso que faz a segunda execução relatar zero inserções e
zero atualizações, e é a propriedade que os testes cobram.

Cuidado com precisão: o Postgres guarda `timestamptz` em microssegundos, e
um instante Go carrega nanossegundos. Comparar sem truncar reporta revisão
em toda reingestão idêntica.

### 7. Registrar a execução, inclusive quando falha

```go
run, _ := ingestion.Begin(ctx, db, srcID, mode, windowStart, windowEnd)
// ...
ingestion.Finish(ctx, db, run.ID, counts)   // ou ingestion.Fail(...)
```

`ingestion_runs` **não é telemetria**: é parte do modelo de dados. Sem ela,
"não houve sismo nesta janela" e "não houve coleta nesta janela" são a mesma
coisa — zero linhas. A §74 proíbe confundir as duas, e a cobertura é
derivada daqui, nunca inferida da tabela de dados.

Uma execução interrompida fica visivelmente em `running`, sem `finished_at`.
Isso é melhor que sumir.

### 8. Nunca derrubar o serviço

O agendador sobe **depois** do listener HTTP, e falha de ciclo é registrada
e engolida. Dado externo é opcional para responder uma requisição; banco não
é. Uma fonte fora do ar não pode parar de servir o que já se tem.

## Incremental e backfill são perguntas diferentes

| | pergunta à fonte | quando usar |
|---|---|---|
| **backfill** | "o que **aconteceu** nesta janela?" | carga histórica inicial |
| **incremental** | "o que você **mudou** desde este instante?" | operação contínua |

Só a segunda revela revisões. É por isso que este projeto usa o FDSN Event
API e não os feeds de tempo real: feed tem janela fixa e um evento corrigido
há duas semanas nunca reaparece nele.

A âncora do ciclo incremental é o fim da última execução bem-sucedida
**menos uma sobreposição de segurança**. Âncora exata perde o evento cujo
`updated` cai entre a fonte responder e a transação commitar. Reingerir o
que já se conhece é de graça — a deduplicação descarta. Perder é silencioso e
permanente. A assimetria é o argumento inteiro.

## Não sobreponha ciclos

`pg_try_advisory_lock`, não mutex em memória. Trava em memória basta para
uma instância e falha calada na segunda — cada processo acha que tem a
trava. O advisory lock está certo nos dois casos e não custa infraestrutura
nova; é o mesmo mecanismo que as migrações já usam.

## Como testar sem rede

A suíte inteira roda sem tocar a internet, e isso é requisito, não
conveniência.

- Respostas reais capturadas uma vez e versionadas em `testdata/`, com um
  README que diz de cada arquivo se é real ou construído.
- O cliente é exercitado contra `httptest.NewServer`, não contra um mock da
  interface HTTP. Mock da interface testa o mock; servidor local exercita
  timeout, código de status e corpo truncado de verdade.
- Testes de integração contra PostGIS real via testcontainers.

Verifique com `docker run --network none ... go test -short -count=1 ./...`.

**A fixture não substitui a coisa real.** Rode o subcomando contra a API
viva ao menos uma vez e confira a contagem contra o que a própria fonte
relata para a mesma janela. Foi assim que apareceu o defeito do `delayed`,
que nenhum teste tinha pego.

## Exemplo executável

`TestIngest_RevisionCreatesAVersionAndAsOfStillSeesTheOldOne`, em
[backend/internal/usgs/ingest_test.go](../backend/internal/usgs/ingest_test.go),
é o teste que dá sentido a esta fase inteira: a fonte revisa um evento, a
revisão vira versão nova, a leitura corrente devolve só a nova, e a consulta
as-of anterior à revisão continua devolvendo a magnitude antiga.

Leia junto com [BITEMPORAL.md](BITEMPORAL.md), que explica o modelo que esse
teste exercita.
