## Why

A V0.1 construiu o armazenamento bitemporal, o registro de fontes e a API — mas
nenhum dado observacional entra no sistema. A tabela `earthquakes` está vazia
desde que foi criada, a garantia as-of nunca foi exercitada contra revisão real
de fonte, e `data_sources` tem uma única linha habilitada. A plataforma sabe
guardar dado científico e não tem nenhum.

A §87 põe "First real data integration" logo depois do registro de fontes, e a
§86 define a V0.2 como primeiras fontes, ingestão, normalização, qualidade de
dados e atualização automática. Esta mudança é essa fase.

O USGS é a primeira fonte por três razões concretas: é domínio público dos EUA
com atribuição, tem API FDSN documentada e estável com o parâmetro
`updatedafter` que torna a ingestão incremental barata, e — o que mais importa
aqui — **revisa eventos publicados**, promovendo-os de `automatic` para
`reviewed` e corrigindo magnitude e profundidade. É exatamente o comportamento
que o armazenamento bitemporal da V0.1 foi construído para representar, e até
hoje só foi testado contra dado fabricado.

## What Changes

- Registro do USGS Earthquake Hazards Program como fonte habilitada, com
  licença, atribuição, cadência e termos, pela mesma disciplina da migração
  `010` — a restrição `data_sources_enabled_requires_license` continua sendo o
  que impede fonte sem licença de ser usada.
- **Adaptador de fonte completo** para o USGS, na cadeia que a §9 exige:
  fetcher → parser → validator → normalizer → banco. Cada etapa é uma unidade
  separada e testável; nenhuma faz o trabalho da outra.
- Ingestão **global**: todos os sismos do catálogo do USGS na janela pedida,
  sem filtro de magnitude nem de proximidade. A associação a vulcões fica por
  conta de consulta geoespacial sobre o dado já guardado, não do que se decide
  ingerir. Ingerir só o que parece relevante hoje é uma decisão que não dá para
  desfazer depois.
- Preservação do **dado cru** (§18): o payload original de cada evento é
  guardado junto do registro normalizado, para que qualquer feature derivada
  possa ser reconstruída a partir da origem.
- Campos que a §9 e a §19 exigem por integração e que a tabela atual não tem:
  status do evento na fonte, tipo de magnitude, indicadores de qualidade da
  solução (`rms`, `gap`, `nst`, `dmin`), versão do parser e versão da fonte.
- **Motor de qualidade de dados** (§20): cada registro ingerido recebe um
  estado — `valid`, `suspect`, `duplicate`, `outlier`, `corrupted` ou `delayed`
  — e o motivo. Registro suspeito é guardado e marcado, nunca descartado em
  silêncio.
- **Normalização** (§21): unidade explícita em tudo, timestamps em UTC,
  coordenadas em SRID 4326, identificadores da fonte preservados sem
  reescrita.
- **Atualização automática**: subcomando `ingest` para operação manual e
  backfill, mais um agendador dentro do backend que chama o mesmo código em
  intervalo configurável. Backfill com janela parametrizável, padrão de 90
  dias.
- **Monitoramento de ingestão** (§73–§75): cada execução registra início, fim,
  contagens, e resultado; falha de fonte é registrada e visível; **ausência de
  dado nunca é interpretada como ausência de atividade** (§74), e lacunas são
  identificáveis.
- Rota da API para ler os sismos ingeridos, com a mesma disciplina temporal e
  de proveniência das rotas existentes.

Fora de escopo, deliberadamente: PVMBG e qualquer outra fonte; associação
sismo↔vulcão como dado persistido; features derivadas, contagem por janela,
energia sísmica acumulada e detecção de swarm (§11) — tudo isso é V0.5 em
diante e depende de baseline que ainda não existe.

**PVMBG fica bloqueado, não adiado por preguiça.** O `robots.txt` de
`magma.esdm.go.id` é `User-agent: * / Disallow: /`, não há API documentada nem
página de termos, e o site declara "All Rights Reserved". O endpoint que
circula publicamente é raspagem de HTML feita por um projeto não-oficial.
Ingerir dali violaria a política declarada do site e a regra deste projeto de
não habilitar fonte sem licença determinada. Desbloquear exige permissão
escrita do PVMBG — é decisão de licenciamento, não tarefa de engenharia.

## Capabilities

### New Capabilities
- `ingestao-fontes`: o contrato de um adaptador de fonte externa — as etapas
  obrigatórias, o que cada execução registra, como falha de fonte é tratada, e
  a garantia de que ausência de dado nunca vira ausência de atividade.
- `qualidade-de-dados`: os estados de qualidade que todo registro ingerido
  recebe, o que cada um significa, e a regra de que registro suspeito é marcado
  e preservado em vez de descartado.
- `sismos`: o que o sistema guarda sobre um sismo, quais atributos são
  obrigatórios, como revisão pela fonte é representada, e como o dado é
  consultável.

### Modified Capabilities
- `registro-de-fontes`: passa a exigir que o registro da fonte carregue também
  a cadência de coleta efetiva e o estado operacional da última execução de
  ingestão — hoje o registro descreve a fonte, mas não diz nada sobre se ela
  está funcionando.
- `api-publica`: nova coleção de sismos sob o contrato existente, e exposição
  do estado de qualidade junto do dado, para que nenhum consumidor leia um
  registro `suspect` como se fosse `valid`.

## Impact

- **Banco:** migrações novas — colunas de qualidade, status, versão de parser e
  dado cru em `earthquakes`; tabela de execuções de ingestão; linha do USGS em
  `data_sources` habilitada com licença. Nenhuma migração existente é alterada:
  as aplicadas são imutáveis.
- **Backend:** pacotes novos para o adaptador USGS, o motor de qualidade e o
  agendador; subcomando `ingest`; rota nova na API. O pacote `observation`
  continua sendo o único lugar autorizado a escrever predicado temporal, e a
  ingestão de sismos passa a ter a mesma disciplina.
- **Rede:** primeira dependência de rede em tempo de execução. Precisa de
  timeout, limite de tentativas, recuo entre tentativas e `User-Agent` que
  identifique o projeto. O ambiente de teste **não** pode depender da rede: o
  adaptador é testado contra servidor local com respostas gravadas da API real.
- **Volume:** o feed global traz na ordem de 10 a 15 mil sismos por mês; um
  backfill de 90 dias fica na casa das dezenas de milhares de linhas, e cresce
  a cada revisão pela fonte, porque revisão é linha nova. Os índices temporais
  da migração `006` já cobrem o padrão de acesso.
- **Documentação:** `docs/DATA_SOURCES.md` ganha a seção do USGS; a decisão
  sobre o PVMBG fica registrada onde a próxima pessoa a procure, em vez de se
  perder nesta proposta.
- **Operação:** o backend passa a ter trabalho de fundo. Falha de ingestão não
  pode derrubar o serviço HTTP nem impedir a subida.
