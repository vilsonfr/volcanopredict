## Why

O repositório hoje contém uma especificação de 2.799 linhas e um esqueleto que **não executa**: falta `frontend/Dockerfile`, `vite.config.ts`, `tsconfig.json`, `.env.example` e `go.sum`, então `docker compose up --build` falha. O backend serve um evento sintético hardcoded como se fosse observação real, violando a §4.1 da master spec. E a tabela `observations` grava apenas `observed_at`, sem registrar *quando o sistema soube* do dado — sem essa coluna, o back-testing exigido pelas §45–47 é impossível de fazer honestamente.

Corrigir a bitemporalidade agora custa uma migração. Corrigir depois de ingerir milhões de observações custa reprocessar tudo. Por isso a V0.1 precisa fechar antes de qualquer conector de fonte externa.

## What Changes

- Fazer o ambiente local subir de ponta a ponta com um único `docker compose up --build`: `frontend/Dockerfile`, `vite.config.ts`, `tsconfig.json`, `.env.example`, `go.sum`, healthcheck do Postgres antes do backend.
- **BREAKING** (schema): reescrever `observations` como tabela bitemporal — separar o instante do fenômeno (`observed_at`) do instante em que o sistema tomou conhecimento (`ingested_at`), com consulta "as-of" garantida por índice. Mesma coisa para `earthquakes`.
- Adicionar marcação obrigatória de proveniência: toda linha de observação e evento carrega `is_synthetic BOOLEAN NOT NULL`, propagado para a resposta da API. Dado sintético nunca sai da API sem essa flag.
- Substituir o migrador implícito do `docker-entrypoint-initdb.d` (que só roda no primeiro boot do volume) por um migrador versionado que roda em todo start e é idempotente.
- Criar o catálogo de vulcões: importador do Smithsonian Global Volcanism Program populando `volcanoes` com dado real, licença e atribuição registradas em `data_sources`.
- Ligar a API ao Postgres: `GET /api/v1/volcanoes` (com filtro geoespacial e paginação) e `GET /api/v1/events` passam a ler do banco. O evento demo hardcoded é removido.
- Enriquecer `data_sources` com licença, termos de uso e cadência de atualização — hoje a tabela não registra nada disso, contra a §115.
- Primeiros testes Go (unitários + integração contra Postgres efêmero) e workflow de CI no GitHub Actions, para satisfazer o critério de conclusão de fase da §88.

Fora de escopo (fases posteriores da §87): globo 3D/CesiumJS, ingestão contínua de fontes externas, baselines, detecção de anomalias, activity score, alertas, machine learning.

## Capabilities

### New Capabilities
- `ambiente-local`: como o projeto sobe, se configura e se migra numa máquina de desenvolvimento — build reprodutível, variáveis de ambiente, ordem de inicialização, migrações versionadas.
- `armazenamento-temporal`: o modelo de dados bitemporal. Como observações e eventos registram tempo do fenômeno vs. tempo de conhecimento, como se consulta o estado "as-of" de um instante passado, e como proveniência real vs. sintética é marcada e nunca perdida.
- `catalogo-vulcoes`: o cadastro de vulcões e sua origem — importação do Smithsonian GVP, identidade estável, reimportação idempotente, rastreabilidade até a fonte.
- `registro-de-fontes`: o catálogo de fontes externas com licença, atribuição, termos de uso e cadência. Pré-condição para qualquer conector futuro.
- `api-publica`: o contrato HTTP v1 — formato de resposta, erros, paginação, filtro geoespacial, e a obrigação de expor proveniência e ressalva científica em todo dado servido.

### Modified Capabilities
<!-- Nenhuma: este é o primeiro conjunto de specs do projeto. -->

## Impact

- **Código:** `backend/cmd/server/main.go` deixa de ser arquivo único e passa a ter camadas (`internal/`); `backend/migrations/` ganha migrações novas e um runner; `frontend/` ganha os arquivos de build que faltam.
- **Schema:** `observations` e `earthquakes` mudam de forma. Não há dado de produção ainda, então a migração pode reescrever as tabelas em vez de fazer backfill.
- **API:** `GET /api/v1/events` muda de formato (ganha `is_synthetic` e envelope de paginação). Nenhum consumidor externo existe ainda.
- **Dependências:** `pgx/v5` (já declarado em `go.mod`, hoje não usado) passa a ser realmente importado; entra um migrador (`goose` ou equivalente).
- **Dados externos:** primeira dependência real do Smithsonian GVP, sujeita aos termos de uso e à atribuição exigida pela fonte.
- **CI:** primeiro workflow do repositório; PRs passam a exigir build e testes verdes.
