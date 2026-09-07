# VolcanoPredict 🌋

Plataforma open source para visualização, análise e pesquisa de atividade vulcânica.

> **Importante:** o projeto não promete prever com certeza a próxima erupção. O objetivo científico é estimar risco/probabilidade, quantificar incerteza e testar hipóteses com dados históricos e quase em tempo real.

## Objetivos da V0.1
- Globo 3D interativo com CesiumJS/React.
- Camadas de vulcões, placas tectônicas e terremotos.
- Arquitetura pronta para cinzas/vento, tsunami e atividade solar.
- Backend em Go.
- PostgreSQL + PostGIS.
- API REST + WebSocket.
- Pipeline de ingestão de fontes externas.
- Sistema de alertas por e-mail configurável.
- Modelo inicial explicável de score de atividade, sem alegar previsão determinística.
- Estrutura preparada para um módulo de IA explicativo no futuro.

## Stack
- Backend: Go
- Frontend: React + TypeScript + Vite + CesiumJS
- Banco: PostgreSQL + PostGIS
- Cache/filas: preparado para Redis
- Containers: Docker Compose
- Testes: Go tests + Vitest
- CI: GitHub Actions

## Rodando
1. Copie `.env.example` para `.env`.
2. Execute `docker compose up --build`.
3. Backend: `http://localhost:8080/health`
4. Frontend: `http://localhost:5173`

## Catálogo de vulcões

O catálogo vem do Smithsonian Global Volcanism Program, versionado em
`backend/data/gvp/` (veja o `MANIFEST.md` de lá para versão, DOI e checksum).

Para importar ou reimportar:

```bash
docker compose exec backend /app/volcanopredict import-catalog
```

O comando verifica o SHA-256 do snapshot contra o manifesto antes de tocar no
banco, e é idempotente: rodar duas vezes seguidas relata zero inserções e zero
atualizações. Vulcões que somem da fonte são marcados como ausentes, nunca
apagados.

Para atualizar o catálogo, baixe o arquivo novo em
<https://volcano.si.edu/volcanolist_holocene.cfm> (o site exige navegador —
clientes automatizados recebem 403 da Cloudflare), substitua o arquivo,
atualize o `MANIFEST.md` com a nova versão, data e SHA-256, e rode o comando.

## Contribuição
Veja `CONTRIBUTING.md`. Procure issues marcadas `good first issue`, adicione testes e cite a fonte dos dados.

## Licença
MIT — veja `LICENSE`.
