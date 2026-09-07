# Verificação final da V0.2 contra a §88 da master spec (tarefa 10.4)

> "Uma fase NÃO deve ser considerada concluída apenas porque o código
> funciona." — §88

Data: 07/09/2026. Cada critério aponta para o artefato que o satisfaz. As
lacunas estão declaradas, não reclassificadas.

## 1. Implementação

| Capacidade | Onde |
|---|---|
| Cliente do FDSN com tentativas, timeout e paginação | `backend/internal/usgs/client.go` |
| Parser do GeoJSON, com ausente ≠ zero | `backend/internal/usgs/parser.go` |
| Orquestração fetch→parse→validate→normalize→persist | `backend/internal/usgs/ingest.go` |
| Motor de qualidade (§20) | `backend/internal/dataquality` |
| Leitura temporal e escrita de sismos | `backend/internal/earthquake` |
| Execuções, cobertura, lacunas e estado operacional | `backend/internal/ingestion/run.go`, `coverage.go` |
| Agendador com advisory lock | `backend/internal/ingestion/scheduler.go` |
| Subcomando `ingest` | `backend/cmd/server/ingest.go` |
| Rotas `/earthquakes` e `/ingestion` | `backend/internal/httpapi/earthquakes.go` |
| Schema: campos de fonte, qualidade, dado cru, execuções | migrações `011`–`015` |

## 2. Testes

**165 funções de teste**, contra 68 no fim da V0.1.

```
dataquality 10 · earthquake 17 · httpapi 27 · ingestion 15 · usgs 29
db 26 · source 9 · observation 9 · gvp 12 · config 7 · volcano 4
```

Suíte completa verde nesta verificação, sem cache (`-count=1`):

```
ok  internal/config       0.002s
ok  internal/dataquality  0.004s
ok  internal/db         100.927s
ok  internal/earthquake  83.211s
ok  internal/gvp         22.795s
ok  internal/httpapi    113.191s
ok  internal/ingestion   75.452s
ok  internal/observation 47.275s
ok  internal/source      48.513s
ok  internal/usgs        67.595s
ok  internal/volcano     22.735s
```

Os testes que mais importam são os de garantia:

- `TestDataLeakage_AsOfNeverSeesFutureKnowledge` (sismos) — verificado por
  **sabotagem deliberada** do predicado `ingested_at <= $1`. A primeira
  tentativa de sabotagem falhava por erro de tipo no SQL, o que não provava
  nada; foi refeita para executar e o teste então falhou na asserção de
  vazamento, que é o que valida o teste.
- `TestIngest_RevisionCreatesAVersionAndAsOfStillSeesTheOldOne` — o teste que
  a V0.1 não tinha como escrever, porque não havia fonte que revisa.
- `TestIngest_CollectionCircumstanceDoesNotFakeARevision` — pinado depois de
  o defeito acontecer de verdade (ver §7 abaixo).
- `TestParse_AbsentIsNotZero` — sobre `ci41531128`, evento real com magnitude
  exatamente 0.0 e `nst`/`dmin`/`gap` ausentes.
- `TestCoverage_UncollectedWindowIsNotSilence` — a §74 em forma executável.

**A suíte inteira roda sem rede.** Verificado com
`docker run --network none ... go test -short -count=1 ./...`, e por inspeção
de que nenhum teste disca para host externo.

## 3. Tratamento de erros

- Falha na etapa de **parse** aborta a resposta inteira, nomeando a etapa:
  importar parte de um documento incompreensível é importar um palpite.
- Falha em **um registro** rejeita só aquele, conta, e segue com os demais.
- Fonte fora do ar: tentativas limitadas com espera crescente, e depois
  `ErrSourceUnavailable` — distinto de falha de parse, para que a execução
  registre "a fonte caiu" e não "a fonte mudou de forma".
- `4xx` **não** é repetido: consulta malformada não melhora sendo repetida.
- Falha de ingestão **nunca** derruba o serviço HTTP nem impede a subida.
- Execução que falha é fechada como falha, com a causa, sem apagar o registro
  da execução anterior bem-sucedida.
- A causa da falha exposta na API é **higienizada**; teste injeta uma causa
  com credencial, caminho de arquivo e URL e afirma que nada disso sai.

## 4. Documentação

- `docs/INGESTION.md` — a anatomia de um adaptador de fonte, escrita para
  quem vai fazer o segundo conector: as etapas, as oito coisas que um
  adaptador precisa fazer, e as armadilhas **já medidas**.
- `docs/DATA_SOURCES.md` — seção do USGS (licença, DOI do ComCat, cadência de
  publicação vs. de coleta) e a seção do bloqueio do PVMBG com a evidência e
  o que exatamente destrava a fonte.
- `README.md` — subcomando `ingest`, agendamento, rotas novas, e o que
  `meta.quality` e `meta.coverage` existem para impedir.
- `backend/testdata/README.md` — de cada fixture, se é real ou construída.
- Comentários de decisão nas migrações e nos pacotes, explicando o porquê.

## 5. Dados de exemplo

- Fixtures **reais** capturadas da API em 07/09/2026, versionadas em
  `backend/testdata/usgs/`, com o README dizendo o que é real e o que foi
  construído a partir de real.
- E, o que importa mais: **dado real de verdade no banco**. A ingestão
  manual da janela 01–03/09 trouxe 642 sismos; o ciclo incremental seguinte
  trouxe mais 1.976. Nenhum dado sintético foi inserido.

## 6. Validação

- **Banco:** `quality_state NOT NULL` sem `DEFAULT`; conjunto de estados
  restrito por `CHECK`; estado não-`valid` exige motivo; `raw` e
  `parser_version` obrigatórios; execução com resultado `failure` exige
  causa; janela de execução não pode ser invertida; `finished_at` presente se
  e somente se a execução terminou; fonte bloqueada não pode ser habilitada.
- **Domínio:** `Provenance` e `dataquality.State` sem zero-value útil; as-of
  futuro rejeitado; coordenada fora de faixa rejeitada; consulta de
  proximidade meio-especificada rejeitada.
- **API:** janela invertida, `as_of` futuro, `quality` inventada, `limit`
  acima do máximo, proximidade incompleta — todos `400` nomeando o parâmetro.

Amostra colhida contra o serviço rodando:

```
GET /api/v1/earthquakes?quality=probably-fine  → 400 (param: quality)
GET /api/v1/earthquakes?lat=-6.1               → 400 (param: lon)
GET /api/v1/earthquakes?from=2026-01-01&to=... → 200, coverage.kind = "none"
GET /api/v1/ingestion                          → 200, healthy = true
```

## 7. Logs

- Cada execução registra modo, janela e as quatro contagens.
- Cada registro rejeitado é logado individualmente, com a causa.
- Cada tentativa de rede repetida é logada com o número da tentativa.
- Ciclo pulado por trava é logado **e** gravado como execução `skipped`.
- O log de requisição da V0.1 continua valendo para as rotas novas.

## 8. README atualizado

Reescrito para a V0.2: seção de ingestão, agendamento, rotas e parâmetros
novos, `meta.quality` e `meta.coverage` explicados pelo que impedem, e a
lista honesta do que **ainda não** existe.

---

## O que a execução real pegou e os testes não

Registrado aqui porque é o achado mais útil desta fase.

1. **Todos os 642 registros do primeiro backfill ficaram `delayed`.** A regra
   de chegada tardia comparava a ocorrência com o agora, e num backfill todo
   registro histórico é trivialmente "atrasado". O campo deixava de
   significar coisa alguma e escondia tudo de quem filtra por `valid`.
   Atraso é propriedade da **coleta**, não do evento.

2. **130 revisões falsas.** Depois de corrigido o item 1, um ciclo
   incremental logo após uma ingestão manual apendou uma versão nova para
   130 eventos que não tinham mudado em nada: o veredito de qualidade virava
   `valid` → `delayed` só porque a coleta era de outro tipo, e a qualidade
   participava da comparação de conteúdo. **Este é o pior defeito possível
   nesta base** — poluir o histórico bitemporal com revisões que a fonte
   nunca fez corrompe exatamente a garantia que o projeto existe para dar.

Nenhum dos dois aparecia em teste nenhum. Ambos apareceram em menos de dez
minutos de uso real, e ambos agora têm teste próprio.

## Conclusão

Os oito critérios estão satisfeitos. Lacunas declaradas:

- **O frontend continua sem teste automatizado**, e o globo ainda não mostra
  os sismos ingeridos. Herdada da V0.1 e não resolvida aqui.
- **A fixture de revisão é construída, não capturada.** Uma revisão do USGS
  é um evento no tempo e não é recuperável retroativamente pela API de
  consulta; duas tentativas de capturar uma ao vivo estão descritas em
  `backend/testdata/README.md`. Os valores usados são a forma que o USGS de
  fato publica. Isso é mitigado — não resolvido — pelo fato de o sistema em
  operação já ter lidado com revisão real.
- **`GET /api/v1/sources` continua sem paginação e serializando a struct Go
  crua**, fora da convenção das demais rotas. Herdada da V0.1.
