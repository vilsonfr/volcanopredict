# Fixtures de teste — NÃO SÃO DADO REAL

Os arquivos deste diretório são fixtures sintéticas escritas à mão para
testar `internal/gvp`. Nenhum deles é um snapshot real do Smithsonian
Global Volcanism Program (GVP) nem de qualquer outra fonte externa.

O snapshot real do GVP não pôde ser baixado nesta fase do projeto (ver
`docs/DATA_SOURCES.md`, seção "Situação da fonte em 2026"). Por isso o
catálogo real de vulcões ainda não foi importado, e não deve ser inferido
a partir destes arquivos.

- `gvp_holocene_fixture.csv`: amostra sintética no formato de colunas que
  o parser de `internal/gvp` espera de um export da Holocene Volcano
  List. Os nomes, países e coordenadas aqui são inventados para exercitar
  o parser e o importador (incluindo um caso de coordenada inválida) —
  nunca devem ser tratados como vulcões reais em nenhum outro contexto.
- `gvp_holocene_malformed.csv`: mesma ideia, mas faltando uma coluna
  obrigatória, para exercitar a falha estrutural explícita do parser.
