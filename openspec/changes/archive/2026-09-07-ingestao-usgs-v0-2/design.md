## Context

Ver `proposal.md` — Why. O que importa aqui é o que já existe e restringe a
solução:

- `earthquakes` já é bitemporal e append-only, com chave natural
  `(external_id, source_id, ingested_at)` e trigger que força `ingested_at`
  (migrações `005`/`009`). A forma de guardar revisão já está resolvida; falta
  quem escreva.
- `internal/observation` é o único lugar autorizado a escrever predicado
  temporal (D7 da V0.1). Sismo hoje não tem equivalente.
- O importador do GVP (`internal/gvp`) é o precedente de ingestão que já existe
  nesta base: verifica checksum antes de tocar no banco, é idempotente, rejeita
  registro inválido individualmente sem abortar o lote, e marca ausência em vez
  de apagar. O adaptador do USGS deve ser reconhecível como parente dele.
- Migrações aplicadas são imutáveis. Tudo aqui é migração nova.
- O ambiente de teste roda sem rede garantida, e a suíte curta roda sem Docker.
- Não há Redis, fila nem worker. Introduzir um agora seria infraestrutura à
  frente da necessidade.

Restrição nova e sem precedente na base: **é a primeira dependência de rede em
tempo de execução.** Tudo que a V0.1 fazia era determinístico a partir de
arquivos versionados.

## Goals / Non-Goals

**Goals:**

- Um adaptador de fonte cuja forma sirva de molde para o segundo e o terceiro
  conector, sem que o segundo precise reescrever a cadeia.
- Exercitar a garantia as-of contra revisão real de fonte, e não contra dado
  fabricado — é o teste que a V0.1 não teve como fazer.
- Ingestão interrompível e retomável: matar o processo no meio não pode deixar
  o banco em estado que a execução seguinte não saiba reconciliar.
- Suíte inteira verde sem acesso à rede.

**Non-Goals:**

- Paralelismo, particionamento ou qualquer otimização de escala. Dezenas de
  milhares de linhas não são um problema de escala.
- Abstração de "fonte genérica" com interface polimórfica antes de existir o
  segundo conector. A forma comum se extrai do segundo, não se adivinha no
  primeiro.
- Associação persistida sismo↔vulcão. É consulta geoespacial sobre o que já
  está guardado.

## Decisions

### D1 — Ingerir do FDSN Event API, não dos feeds GeoJSON de tempo real

Os feeds (`summary/all_hour.geojson` etc.) são mais simples e atualizam a cada
minuto, mas têm janela fixa e **não expõem revisão**: um evento corrigido há
duas semanas não aparece em `all_day`. O FDSN Event API aceita `updatedafter`,
que devolve exatamente os eventos cujo registro mudou desde um instante — que é
a pergunta que a ingestão incremental precisa fazer para um banco bitemporal.

Alternativa considerada: feeds para o ciclo curto e FDSN para reconciliação
periódica. Rejeitada por ora — dois caminhos de código para o mesmo dado, e a
diferença de latência (minutos) não importa numa fase sem alertas.

### D2 — `updatedafter` ancorado na última execução bem-sucedida, com recuo

A janela de cada ciclo incremental é `updatedafter = fim da última execução
bem-sucedida menos uma sobreposição de segurança`. A sobreposição existe porque
âncora exata perde evento cujo `updated` cai no intervalo entre a consulta e o
commit. Reingerir evento já conhecido é barato e não cria versão nova; perdê-lo
é silencioso e permanente.

Ancorar no relógio local em vez de no da fonte seria mais simples e erraria
sempre que os dois divergissem.

### D3 — Deduplicação por comparação de conteúdo, não por confiança no `updated`

Uma versão nova só é escrita quando o conteúdo normalizado do evento difere da
versão corrente. O campo `updated` da fonte é registrado, mas não é o que
decide: fonte que reemite payload idêntico com `updated` novo criaria uma versão
por ciclo, inflando o histórico com revisões que não revisaram nada.

Isso preserva a propriedade que o importador do GVP já tem e que os testes
cobram: **segunda execução sem mudança na fonte relata zero inserções e zero
atualizações.**

### D4 — Dado cru guardado como recebido, ao lado do normalizado

O payload de cada evento é persistido junto da linha normalizada, na mesma
transação. Guardar em tabela separada economizaria espaço em troca de um join
em toda reconciliação e da possibilidade de as duas metades divergirem.

A versão do parser fica gravada em cada linha. Sem ela, um bug de parse
corrigido depois torna impossível saber quais linhas nasceram errado — e a
§54 (reprodutibilidade) deixa de valer.

### D5 — Qualidade avaliada na ingestão e gravada, não calculada na leitura

O estado de qualidade é atributo da versão, decidido no momento em que ela é
escrita, com as regras vigentes naquele momento. Calcular na leitura faria o
passado mudar sempre que uma regra mudasse — exatamente o que o modelo
bitemporal existe para impedir.

O preço: mudar uma regra de qualidade não reavalia o histórico. É o preço
certo. Reavaliação futura, se necessária, será ingestão nova com versão de
regras nova, não `UPDATE`.

### D6 — Cobertura de ingestão derivada das execuções, não inferida da ausência

"Houve coleta nesta janela?" é respondido pela tabela de execuções, não pela
presença de linhas em `earthquakes`. Inferir cobertura da ausência de dado é
literalmente o erro que a §74 proíbe: sem essa tabela, janela sem sismo e
janela sem coleta são indistinguíveis.

Isso torna a tabela de execuções parte do modelo de dados, não telemetria
descartável.

### D7 — Agendador in-process, com trava no banco

Uma goroutine no backend dispara o ciclo em intervalo configurável, e a
não-sobreposição é garantida por advisory lock no Postgres — o mesmo mecanismo
que as migrações já usam. Trava em memória bastaria para uma instância e
falharia silenciosamente na segunda; o advisory lock funciona nos dois casos e
não custa infraestrutura nova.

Alternativas: serviço separado no compose (adia para quando houver o que
escalar) e cron externo (deixa a "atualização automática" da §86 fora do
sistema, dependente de configuração de ambiente não versionada).

O agendador é uma casca fina: recebe o mesmo caso de uso que o subcomando
chama. Nada de lógica de ingestão vive dentro dele, para que a suíte teste a
ingestão sem esperar relógio.

### D8 — Falha de ingestão nunca impede subir nem servir

O agendador sobe depois do listener HTTP e sua falha não propaga para o
processo. Uma fonte inacessível na subida produz execução registrada com falha
e um serviço que atende normalmente — o oposto do banco indisponível, que
legitimamente impede servir. Dado externo é opcional para responder; banco não.

### D9 — Testes contra servidor HTTP local com respostas gravadas da API real

Toda a suíte roda sem rede. As respostas são capturadas uma vez da API real,
versionadas em `testdata` e marcadas como amostra — inclusive uma sequência do
mesmo evento antes e depois de revisão pelo USGS, que é o caso que dá sentido a
esta fase.

Mock da interface HTTP testaria o mock. Servidor local exercita o cliente de
verdade: timeout, código de status, corpo truncado, JSON malformado.

Cuidado herdado da V0.1: a fixture não substitui a coisa real. Uma verificação
manual contra a API viva, com a saída colada no relatório da tarefa, continua
sendo obrigatória — mas fora da suíte automatizada.

### D10 — `internal/earthquake` espelha `internal/observation`

Sismo ganha seu próprio pacote, único autorizado a escrever predicado temporal
sobre `earthquakes`, com a mesma disciplina: `Provenance` sem zero-value útil,
as-of futuro rejeitado, `ingested_at` inexistente no tipo de escrita.

Generalizar `observation` para servir às duas tabelas foi considerado e
rejeitado: as chaves naturais são diferentes, e a generalização prematura
esconderia a diferença atrás de um parâmetro. A duplicação é pequena, visível e
comparável lado a lado — e a suíte cobra as duas.

## Risks / Trade-offs

- **A API do USGS muda de forma sem aviso** → o parser falha explicitamente em
  vez de adivinhar; a execução fica registrada como falha; a fixture versionada
  documenta a forma esperada num instante datado.
- **Backfill de 90 dias enche o banco com dado que ninguém consulta ainda** →
  dezenas de milhares de linhas com os índices da migração `006` já cobrindo o
  acesso; a janela é parâmetro, e um operador pode começar menor.
- **Ingestão interrompida no meio** → cada lote é uma transação; a execução
  seguinte recobre a janela pela âncora com sobreposição (D2) e a deduplicação
  por conteúdo (D3) impede versão duplicada. O custo de interromper é repetir
  trabalho, nunca corromper.
- **O relógio da fonte e o nosso divergem** → `occurred_at` e `updated` vêm da
  fonte; `ingested_at` é sempre nosso, imposto por trigger. A garantia as-of
  depende só do nosso relógio.
- **Consultar a fonte com frequência demais** → intervalo configurável com
  padrão conservador, tentativas limitadas com espera crescente, `User-Agent`
  que identifica o projeto. Uma plataforma científica que sobrecarrega a fonte
  de que depende não merece o dado.
- **Sobreposição de segurança grande demais reingere muito; pequena demais
  perde evento** → o teste que importa não é o do tamanho da janela, e sim o de
  que nenhum evento revisado se perde entre dois ciclos consecutivos.

## Migration Plan

Migrações novas, em ordem, sem alterar nenhuma existente:

1. Colunas em `earthquakes`: status da fonte, tipo de magnitude, indicadores de
   qualidade da solução, estado de qualidade (`NOT NULL`, sem `DEFAULT`, pela
   mesma disciplina de `is_synthetic`), motivo, versão do parser, versão da
   fonte e dado cru. A tabela está vazia em todo ambiente — é a última vez que
   isso é verdade, e o momento certo de fazer isso sem retrocompatibilidade.
2. Tabela de execuções de ingestão, com índice por fonte e instante.
3. Registro do USGS em `data_sources` com licença, atribuição, termos, cadência
   de publicação e cadência de coleta, habilitando a fonte — espelhando a `010`.
4. Colunas de estado operacional e política de coleta em `data_sources`, e
   registro do bloqueio do PVMBG com o motivo.

Rollback: cada migração tem `Down`. O dado ingerido não é apagado por rollback
de schema; desfazer a fase significa desabilitar a fonte, não destruir
histórico (§89.12).

Ordem de implementação: cliente e parser contra fixture → validação e
normalização → qualidade → persistência com deduplicação → execuções e
cobertura → subcomando → agendador → API. Cada etapa é verificável sozinha,
e nenhuma depende do agendador para ser testada.

## Open Questions

- Qual o valor padrão do intervalo de coleta automática. Não muda spec, design
  nem tarefas: é configuração, e a decisão fica melhor com uma execução real
  medida do que agora.
- Se a cobertura de ingestão deve ser exposta como campo do envelope em toda
  resposta temporal ou apenas nas rotas de dado ingerido. A spec exige que seja
  observável; onde exatamente ela aparece é detalhe de serialização, resolvível
  na tarefa da API.
