# Fixtures de teste

Nada aqui é catálogo. Estes arquivos existem para exercitar parsers e
importadores, e a suíte inteira roda sem tocar a rede por causa deles.

Cada arquivo abaixo diz explicitamente se é **dado real capturado** ou
**dado construído**. A distinção não é formalidade: tratar uma fixture como
observação é a forma mais fácil de inventar dado sem perceber (§89.3).

## `gvp_*` — construídas à mão

Sintéticas, escritas para exercitar `internal/gvp`. Os nomes, países e
coordenadas são inventados. **Não são vulcões reais** e não substituem o
snapshot do GVP — esse é real, está versionado em `backend/data/gvp/`, e é
o que a importação de verdade usa.

- `gvp_holocene_fixture.xls` — amostra no formato SpreadsheetML que o
  parser espera, incluindo um registro com coordenada inválida.
- `gvp_holocene_malformed.xls` — o mesmo, sem uma coluna obrigatória, para
  exercitar a falha estrutural explícita.

## `usgs/` — capturadas da API real

Baixadas do FDSN Event Web Service em **07/09/2026**, com o `User-Agent`
deste projeto. São respostas reais, byte a byte, exceto onde dito.

- **`query_m5_2026-09-01_03.json`** — real, verbatim. 15 eventos M5+ entre
  01 e 03 de setembro de 2026. É a fixture principal do parser.
- **`query_absent_vs_zero.json`** — real, verbatim. Um único evento,
  `ci41531128`, que é o caso mais útil deste diretório: tem **magnitude
  exatamente 0.0** e omite `nst`, `dmin` e `gap` por completo. Prova, com
  dado real, que ausente e zero são fatos diferentes — nas duas direções.
  Um parser que decodifique em `float64` simples lê o zero medido e os três
  campos ausentes de forma idêntica.
- **`query_with_bad_records.json`** — **construída** a partir de features
  reais do arquivo M5+ acima. Duas features foram deliberadamente
  quebradas (uma sem `time`, outra sem `id`) para exercitar a rejeição
  individual sem abortar o lote. As duas features boas são reais.
- **`malformed_not_a_collection.json`** e **`malformed_truncated.json`** —
  construídas. Um documento que não é `FeatureCollection` e um JSON cortado
  no meio, para exercitar a falha explícita na etapa de parse.

### Sobre a revisão de eventos

Os testes de revisão em `internal/usgs/ingest_test.go` usam documentos
**construídos**, não uma captura de um antes-e-depois real.

Isso é uma limitação declarada, e vale explicar por quê. Uma revisão do
USGS é um evento no tempo: para capturar os dois lados é preciso já estar
observando quando ela acontece. Consultar o catálogo hoje devolve só o
estado atual — o anterior não é recuperável pela API de consulta.

Duas tentativas de contornar isso e por que não serviram:

1. O feed de detalhe de um evento traz `products.origin[]` com várias
   versões. Parecia um histórico de revisão, mas ao inspecionar
   `us7000tdmm` as duas versões eram de **redes diferentes** (`us`, com
   5.2 mb, e `ak`, com 5.0 ml) — soluções concorrentes, não uma correção
   sucessiva da mesma solução. Rotular como revisão seria falsear.
2. Fotografar eventos recentes e reconsultar em ciclos, esperando pegar uma
   promoção `automatic → reviewed` ao vivo. Rodou durante esta sessão sem
   capturar nenhuma.

Os valores usados nos testes construídos são, ainda assim, a forma que o
USGS de fato publica: magnitude e tipo mudando juntos (5.2 mb → 5.0 ml),
profundidade recalculada, e `status` promovido de `automatic` para
`reviewed` — documentado em `docs/DATA_SOURCES.md`. Quando alguém capturar
uma revisão real, trocar a fixture e apagar esta seção.

## Como recapturar

```bash
curl -A "volcanopredict/0.2 (+https://github.com/vilsonfr/volcanopredict)" \
  "https://earthquake.usgs.gov/fdsnws/event/1/query?format=geojson&starttime=...&endtime=...&minmagnitude=5"
```

Ao trocar uma fixture real, atualize a data acima e confira se os testes
que afirmam valores concretos (magnitude 6.2 de `us7000tdrv`, o zero de
`ci41531128`) ainda batem — eles são deliberadamente específicos.
