# VolcanoPredict 🌋

Plataforma open source para visualização, análise e pesquisa de atividade
vulcânica.

> **Ressalva científica.** Este projeto **não prevê erupções**. Ele não afirma
> que uma erupção vai ocorrer, nem quando. O objetivo é estimar risco e
> probabilidade a partir de dados observacionais, quantificar a incerteza dessa
> estimativa e permitir testar hipóteses contra o histórico. Qualquer score
> produzido aqui é experimental e explicável, nunca uma previsão oficial —
> alertas oficiais vêm dos observatórios vulcanológicos responsáveis
> (USGS VHP, PVMBG, e os demais). Dado real e dado sintético nunca são
> misturados: cada observação carrega a marca da sua proveniência, e a API
> devolve apenas dado real por padrão.

## Estado atual (V0.1)

O que já funciona:

- Ambiente completo por `docker compose`, de clone limpo, com migrações
  versionadas aplicadas na subida.
- Armazenamento **bitemporal** append-only: cada dado sabe quando o fenômeno
  aconteceu e quando o sistema ficou sabendo dele. Ver
  [docs/BITEMPORAL.md](docs/BITEMPORAL.md).
- Catálogo de 1.214 vulcões do Holoceno, do Smithsonian Global Volcanism
  Program, importado de forma idempotente.
- API REST versionada com envelope, paginação, consulta as-of e filtro
  geoespacial.
- Globo 3D com os vulcões do catálogo, busca, legenda e detalhe por vulcão.

O que ainda **não** existe: conectores de ingestão contínua (sismos, alertas,
satélite), WebSocket, alertas por e-mail e qualquer modelo de score. Isso é a
V0.2 em diante.

## Stack

Go no backend, PostgreSQL + PostGIS no banco, React + TypeScript + Vite +
CesiumJS no frontend, tudo orquestrado por Docker Compose. CI no GitHub
Actions.

## Subindo o projeto

Pré-requisito: Docker com Compose v2. Nada mais — não é preciso ter Go nem
Node instalados.

```bash
git clone https://github.com/vilsonfr/volcanopredict.git
cd volcanopredict
cp .env.example .env
docker compose up --build
```

Quando terminar de subir:

- Backend: <http://localhost:8080/health> — responde `200` só quando o banco
  está acessível e o schema está na versão esperada.
- Frontend: <http://localhost:5173>

Se alguma dessas portas já estiver ocupada na sua máquina (uma instalação local
de PostgreSQL segurando a 5432 é o caso mais comum), ajuste `DB_HOST_PORT`,
`BACKEND_HOST_PORT` e `FRONTEND_HOST_PORT` no `.env`. Ao mudar
`BACKEND_HOST_PORT`, ajuste também `VITE_API_BASE_URL`, que é a URL usada pelo
navegador — de fora da rede do compose.

O globo funciona sem nenhuma chave. Um token gratuito do
[Cesium Ion](https://ion.cesium.com/tokens) em `VITE_CESIUM_ION_TOKEN` ativa o
terreno com relevo real; sem ele, a superfície é uma esfera lisa com imagem de
satélite.

## Importando o catálogo de vulcões

O catálogo vem do Smithsonian Global Volcanism Program e está versionado em
[backend/data/gvp/](backend/data/gvp/) — veja o `MANIFEST.md` de lá para versão
da base, DOI, data do download e checksum.

```bash
docker compose exec backend /app/volcanopredict import-catalog
```

O comando verifica o SHA-256 do snapshot contra o manifesto antes de tocar no
banco. É idempotente: rodar de novo sem o arquivo ter mudado relata zero
inserções e zero atualizações, e os identificadores dos vulcões permanecem
estáveis. Registro com coordenada inválida é rejeitado individualmente, com log,
sem abortar o resto da importação. Vulcão que sumiu da fonte é **marcado como
ausente, nunca apagado**.

Para atualizar o catálogo, veja o procedimento em
[docs/DATA_SOURCES.md](docs/DATA_SOURCES.md).

## API

Todas as rotas ficam sob `/api/v1/`. Caminho de API sem prefixo de versão
responde `404` — a versão nunca é implícita.

| Rota | Descrição |
|---|---|
| `GET /health` | conectividade com o banco e versão de schema |
| `GET /api/v1/volcanoes` | catálogo, com busca, filtros e proximidade |
| `GET /api/v1/observations` | observações, com janela temporal e consulta as-of |
| `GET /api/v1/sources` | fontes registradas, com licença e atribuição |

Toda resposta usa o mesmo envelope — `data`, `meta`, `attribution` — e
`meta.disclaimer` acompanha todo valor derivado, sem forma de suprimi-lo por
parâmetro ou cabeçalho. Erros têm formato único, com código estável e um
identificador de correlação que também aparece no log estruturado.

Parâmetros principais:

- **Paginação** (em `/volcanoes` e `/observations`): `limit` (padrão 100,
  máximo 500 — acima disso é `400`, não um corte silencioso) e `cursor` opaco,
  por keyset. `/sources` devolve a lista inteira, que é pequena e fixa.
- **`/volcanoes`:** `q` (busca por nome), `country`, `status`,
  `include_absent=true`; e `lat` + `lon` + `radius_km` juntos para consulta por
  proximidade, que devolve a distância em cada item, ordenada crescente. Os três
  são tudo-ou-nada: meia consulta de proximidade é `400`.
- **`/observations`:** `volcano_id`, `from`/`to` (janela sobre o instante do
  fenômeno), `as_of` (instante de conhecimento; futuro é `400`) e `provenance`
  (`real`, `synthetic` ou `all`; o padrão é `real`).

Alguns exemplos:

```bash
curl 'http://localhost:8080/api/v1/volcanoes?q=etna'
curl 'http://localhost:8080/api/v1/volcanoes?lat=-6.1&lon=106.8&radius_km=300'
curl 'http://localhost:8080/api/v1/observations?as_of=2026-03-15T00:00:00Z'
```

## Rodando os testes

Os testes de integração sobem um PostGIS efêmero via `testcontainers-go`, então
a suíte completa precisa de um daemon do Docker acessível. A suíte curta não
precisa de nada.

Com Go instalado:

```bash
cd backend
go test -short ./...   # unitários, sem Docker
go test ./...          # tudo, sobe containers
```

Sem Go instalado, pela imagem oficial:

```bash
cd backend

# Unitários
docker run --rm -v "$PWD":/app -w /app -u "$(id -u):$(id -g)" \
  golang:1.24-alpine sh -c 'go build ./... && go vet ./... && go test -short ./...'

# Integração: precisa do socket do Docker e do gid do grupo docker do host
docker run --rm -v "$PWD":/app -w /app -u "$(id -u):$(id -g)" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  --group-add "$(getent group docker | cut -d: -f3)" \
  -e TESTCONTAINERS_RYUK_DISABLED=true \
  golang:1.24-alpine sh -c 'go test ./...'
```

Alternativamente, apontar `TEST_DATABASE_URL` para um Postgres+PostGIS já
rodando faz os testes usarem esse banco em vez de subir containers.

O frontend:

```bash
cd frontend && npm ci && npm run typecheck && npm run build
```

O `typecheck` é um passo separado porque o build do Vite só transpila: erro de
tipo passa direto por ele.

O CI ([.github/workflows/ci.yml](.github/workflows/ci.yml)) roda exatamente
isso: build, `go vet`, suíte curta, suíte completa com containers, e typecheck
mais build do frontend.

## Documentação

- [docs/BITEMPORAL.md](docs/BITEMPORAL.md) — o modelo bitemporal e como
  escrever uma consulta as-of sem vazar conhecimento do futuro. **Leitura
  obrigatória antes de mexer em qualquer consulta temporal.**
- [docs/DATA_SOURCES.md](docs/DATA_SOURCES.md) — cada fonte, sua licença,
  atribuição, cadência e versão de snapshot.
- [VOLCANO_PREDICTION_MASTER_SPEC.md](VOLCANO_PREDICTION_MASTER_SPEC.md) — a
  especificação completa do projeto.
- [openspec/](openspec/) — as mudanças em andamento e seus specs.

## Contribuição

Veja [CONTRIBUTING.md](CONTRIBUTING.md). Procure issues marcadas
`good first issue`, adicione testes e cite a fonte dos dados. Duas regras que
não se negociam: não apagar dado histórico, e não misturar dado real com
sintético.

## Licença

MIT — veja [LICENSE](LICENSE). Os dados do Smithsonian GVP têm termos próprios,
descritos em [docs/DATA_SOURCES.md](docs/DATA_SOURCES.md).
