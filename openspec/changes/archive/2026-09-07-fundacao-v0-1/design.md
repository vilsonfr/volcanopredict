## Context

Ver `proposal.md` — Why. Os requisitos estão em `specs/`.

Restrições que moldam o desenho:

- **O código atual é um esqueleto.** `backend/cmd/server/main.go` é arquivo único, sem acesso a banco. Não há dado de produção, nenhum consumidor externo da API e nenhum teste. Isso dá liberdade total de refatoração agora — liberdade que desaparece assim que a primeira ingestão real rodar.
- **A fonte do catálogo está em transição.** O Smithsonian GVP descontinuou as funções de busca do site e a API enquanto prepara um site novo ao longo de 2026. O que permanece disponível é o download em massa da *Holocene Volcano List* em XML e o produto VAAC, com atualizações significativas anuais (tipicamente até início de junho) e menores a cada 6–8 semanas. Não existe endpoint estável para consultar programaticamente hoje.
- **A master spec proíbe inventar APIs** (§89, itens 4 e 5) e exige que dado sintético jamais se misture a dado real (§4.1).
- **§88 exige que uma fase só feche com testes, tratamento de erro, documentação e logs** — não basta o código rodar.

## Goals / Non-Goals

**Goals:**

- Modelo bitemporal correto na primeira migração, porque corrigi-lo depois de ingerir dados custa reprocessamento total.
- Ambiente reprodutível: `docker compose up --build` a partir de clone limpo, sem Go/Node/Postgres no host.
- Importação do catálogo determinística e auditável, sem depender de disponibilidade de site em obras.
- Fronteira clara entre camadas para que os conectores da V0.2 entrem sem reescrever nada.

**Non-Goals (nível de desenho):**

- Não desenhar o pipeline de ingestão contínua nem agendamento — a V0.1 importa sob demanda.
- Não introduzir Redis, filas ou workers assíncronos. A §93/§94 preveem isso, mas não há carga que justifique agora.
- Não escolher framework HTTP nem ORM. `net/http` e SQL direto bastam para cinco endpoints, e evitam acoplar cedo.
- Não desenhar autenticação: a API é pública e somente leitura nesta fase.

## Decisions

### D1 — Bitemporalidade por append-only, não por intervalos de validade

Cada tabela temporal ganha `observed_at` (tempo do fenômeno) e `ingested_at` (tempo de conhecimento, `DEFAULT now()`, sem escrita pelo cliente). Registros nunca sofrem `UPDATE` nem `DELETE`; uma revisão da fonte entra como linha nova com o mesmo identificador natural e `ingested_at` maior. A leitura corrente é "a linha de maior `ingested_at` por identificador natural"; a leitura as-of é a mesma coisa restrita a `ingested_at <= T`.

**Por que, e não SCD2 com `valid_from`/`valid_to`:** SCD2 exige escrever na linha anterior para fechar seu intervalo, o que reintroduz `UPDATE` numa tabela que a §89 item 12 quer imutável, e cria uma janela de inconsistência entre o fechamento e a inserção. Append-only torna a consulta as-of um predicado puro sobre uma coluna imutável — o que é exatamente o que o teste de data leakage da §81 precisa verificar. O custo é que toda leitura corrente precisa de um `DISTINCT ON`/window function em vez de um filtro simples; aceito, e mitigado por índice.

**Descartado também:** extensão `temporal_tables` do Postgres — adiciona dependência não disponível na imagem `postgis/postgis`, e esconde a semântica temporal atrás de triggers, dificultando auditar exatamente o que a §4.3 proíbe.

### D2 — Chave natural explícita por tabela temporal

Cada tabela temporal declara qual tupla de colunas identifica "a mesma coisa do mundo" ao longo das revisões (para sismos, o identificador da fonte; para observações, `volcano_id + kind + observed_at + source_id`). A unicidade é `(chave natural, ingested_at)`, não a chave natural sozinha.

**Por que:** sem isso, "a versão mais recente" é indefinida e a ingestão duplicaria em vez de revisar. Deixar implícito é o erro que só aparece na primeira reingestão.

### D3 — Migrações embutidas no binário, aplicadas no start

Substituir `docker-entrypoint-initdb.d` por `goose` usado como biblioteca, com os arquivos SQL embutidos via `embed.FS`. O backend aplica migrações pendentes ao iniciar, dentro de transação por migração, antes de abrir o listener HTTP.

**Por que, e não `docker-entrypoint-initdb.d`:** aquele diretório só executa no primeiro boot do volume. Toda migração adicionada depois é silenciosamente ignorada em qualquer ambiente já inicializado — falha que não dá erro, só divergência de schema. **Por que goose e não golang-migrate ou Atlas:** goose roda como biblioteca com `embed.FS` sem container ou binário extra; golang-migrate exige mais fiação para o mesmo resultado; Atlas resolve versionamento declarativo que não é problema nosso ainda. **Por que não migrar em container separado:** um serviço a mais no compose para uma operação que precisa acontecer exatamente antes do backend servir — acoplar ao start elimina a ordenação frágil.

Consequência aceita: com múltiplas réplicas do backend, o start concorrente disputaria a migração. Resolvido pelo lock de advisory do próprio goose; irrelevante em V0.1 (réplica única) mas correto desde já.

### D4 — Catálogo importado de snapshot versionado, não de download ao vivo

O arquivo bruto do GVP é baixado manualmente, commitado em `backend/data/gvp/` junto com sua versão da base, DOI, data de download e checksum SHA-256. O importador lê o arquivo local. Atualizar o catálogo é um ato deliberado: baixar o novo snapshot, commitar, rodar a importação.

**Por que, e não baixar ao vivo na subida:** a fonte está em obras e sem endpoint estável — construir contra ela hoje é exatamente a suposição que a §89 item 5 proíbe. Além disso, snapshot versionado dá reprodutibilidade (§54): qualquer pessoa que faça checkout de um commit obtém o mesmo catálogo que produziu aqueles resultados. Um download ao vivo faria o resultado de um back-test depender do dia em que rodou.

Trade-off: o catálogo envelhece até alguém atualizar o snapshot. Aceitável — a cadência da fonte é de meses, e a V0.1 não tem ingestão contínua.

### D5 — Identidade do vulcão derivada do número GVP, com origem preservada

`volcanoes` ganha `source_id` (fonte no catálogo) e `source_ref` (o número de vulcão do GVP), com unicidade em `(source_id, source_ref)`. A chave primária interna segue sendo sintética.

**Por que não usar o número GVP como chave primária:** amarraria todo o banco a uma fonte específica. A V0.2 vai trazer vulcões de PVMBG e USGS que precisarão coexistir e eventualmente ser reconciliados com o registro do GVP; chave sintética permite isso sem migração destrutiva.

Vulcões que somem da fonte recebem `absent_from_source_at` em vez de `DELETE`, conforme a spec do catálogo e a §89 item 12.

### D6 — `is_synthetic BOOLEAN NOT NULL`, sem DEFAULT

Coluna obrigatória em `observations` e `earthquakes`, sem valor padrão, de modo que um `INSERT` que a omita falhe no banco e não apenas na aplicação.

**Por que sem DEFAULT:** um `DEFAULT false` transforma esquecimento em afirmação de que o dado é real — precisamente a mistura silenciosa que a §4.1 proíbe. Fazer a omissão quebrar é a garantia mais barata disponível, e vale mesmo para escrita feita por script fora da aplicação.

### D7 — Camadas em `internal/`, sem framework

```
backend/
  cmd/server/          composição e start
  internal/config/     leitura e validação de env (falha rápido se faltar)
  internal/db/         pool pgx, migrações embutidas, healthcheck
  internal/volcano/    domínio + repositório do catálogo
  internal/observation/domínio + repositório temporal (as-of vive aqui)
  internal/source/     registro de fontes
  internal/gvp/        parser e importador do snapshot
  internal/httpapi/    handlers, envelope de resposta, erros, middleware
  data/gvp/            snapshot versionado + checksum
```

A consulta as-of fica confinada a `internal/observation`. Nenhum handler monta SQL temporal.

**Por que confinar:** o teste de data leakage da §81 precisa de um ponto único onde a regra "só o que era conhecido em T" é aplicada. Se a lógica temporal vazar para handlers, cada endpoint novo vira uma chance de reintroduzir vazamento.

### D8 — Paginação por keyset, não por offset

Listagens paginam por cursor opaco sobre `(chave de ordenação, id)`, com ordenação total sempre incluindo `id` como desempate.

**Por que não `LIMIT/OFFSET`:** offset degrada em tabelas grandes — e `earthquakes` cresce para milhões já na V0.2 — e permite repetir ou pular itens quando há escrita concorrente, violando o requisito de paginação determinística.

### D9 — Ressalva científica no envelope, não no handler

Toda resposta usa um envelope comum (`data`, `meta`, `attribution`); respostas que contenham valor derivado carregam `meta.disclaimer` preenchido pela camada de serialização, não por cada handler.

**Por que:** a §85 e a spec da API exigem a ressalva em todo resultado derivado. Deixar cada handler lembrar de incluí-la garante que um dia alguém esqueça. No envelope, esquecer é impossível.

### D10 — Testes de integração contra Postgres real, não mock

Testes de repositório sobem um PostGIS efêmero via `testcontainers-go`, com `go test -short` pulando-os para a máquina do dia a dia e o CI rodando o conjunto completo.

**Por que não mockar o banco:** metade das regras desta change *é* comportamento de banco — `NOT NULL` sem default, unicidade da chave natural, uso de índice na consulta as-of, distância geográfica do PostGIS. Um mock daria verde exatamente onde os defeitos moram.

## Risks / Trade-offs

- **A consulta as-of com `DISTINCT ON` degrada conforme o histórico de revisões cresce** → índice em `(chave natural, ingested_at DESC)` e teste que inspeciona o plano de execução, exigido pela spec de armazenamento temporal. Se degradar mesmo assim, o caminho é uma view materializada da versão corrente — mas só com medição que justifique.
- **Snapshot do GVP envelhece silenciosamente** → o registro da fonte guarda a versão e a data do snapshot, e a API expõe isso na atribuição. Fica visível em vez de invisível.
- **O formato XML do GVP pode mudar quando o site novo entrar no ar** → o parser valida contra o checksum do snapshot conhecido e falha explicitamente diante de estrutura inesperada, em vez de importar registros parciais. A troca de formato vira uma change própria.
- **`testcontainers-go` exige Docker disponível no runner de CI e encarece o build** → aceitável; a alternativa (mock) invalidaria os testes que mais importam. `-short` mantém o loop local rápido.
- **Append-only faz o banco crescer mais rápido que o volume de fatos** → aceito conscientemente: a §89 item 12 proíbe apagar histórico, e o custo de armazenamento é irrelevante frente ao valor do back-testing. Retenção, se um dia necessária, é decisão científica e não operacional.
- **Refatorar `main.go` em camadas antes de existir qualquer teste** → a change entrega os testes junto; a ordem em `tasks.md` põe o ambiente e os testes de fumaça antes da refatoração, para que a quebra apareça.

## Migration Plan

Não há dado de produção nem consumidor da API, então não há backfill nem compatibilidade a preservar.

1. Migrações novas recriam `observations` e `earthquakes` na forma bitemporal. Como as tabelas estão vazias em todo ambiente existente, `DROP`/`CREATE` é aceitável — e é a última vez que será, pois a partir da primeira ingestão real a regra da §89 item 12 passa a valer integralmente.
2. As migrações `001` e `002` permanecem intactas no repositório (D3 e a spec de imutabilidade); as mudanças entram como `003` em diante.
3. Ambientes de desenvolvimento existentes: como o schema antigo veio de `docker-entrypoint-initdb.d`, o registro de versões do goose estará vazio. A migração `003` é escrita para ser tolerante a esse estado — `CREATE TABLE IF NOT EXISTS` e `DROP TABLE IF EXISTS` — de modo que tanto um volume novo quanto um volume antigo convirjam para o mesmo schema.
4. Rollback: descartar o volume e subir de novo. Válido apenas nesta fase, e deixa de ser a partir da V0.2.

## Open Questions

- Qual identificador de licença registrar para o GVP: os termos permitem redistribuição do snapshot dentro do repositório, ou apenas uso com atribuição? Precisa de leitura dos termos antes de commitar o arquivo bruto. Não muda o desenho — muda apenas se `backend/data/gvp/` guarda o arquivo ou apenas o checksum com instrução de download.
- Se ficará a cargo desta fase importar também a lista Pleistocene, além da Holocene. A spec não distingue; a Holocene cobre o escopo científico da V0.1 e a Pleistocene pode entrar depois sem mudança de schema.
