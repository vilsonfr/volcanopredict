# Verificação final da V0.1 contra a §88 da master spec (tarefa 7.6)

> "Uma fase NÃO deve ser considerada concluída apenas porque o código
> funciona." — §88

Data da verificação: 07/09/2026. Cada critério abaixo aponta para o artefato
que o satisfaz. Onde há lacuna, ela está declarada, não escondida.

## 1. Implementação

| Capacidade | Onde |
|---|---|
| Configuração validada por ambiente | `backend/internal/config` |
| Conexão, espera pelo banco e migrações com advisory lock | `backend/internal/db`, `backend/migrations` (010 migrações) |
| Schema bitemporal append-only | migrações `004`–`009` |
| Leitura temporal (corrente, as-of, proveniência) | `backend/internal/observation` |
| Registro de fontes | `backend/internal/source` |
| Parser e importador do catálogo GVP | `backend/internal/gvp` |
| Catálogo e consulta geoespacial | `backend/internal/volcano` |
| API v1 com envelope, paginação e erros | `backend/internal/httpapi` |
| Subcomando de importação | `backend/cmd/server/import.go` |
| Globo 3D, busca, filtros, legenda e detalhe | `frontend/src` |

## 2. Testes

68 funções de teste em Go, distribuídas por todos os pacotes com lógica:

```
config 7 · db 18 · gvp 12 · httpapi 15 · observation 9 · source 3 · volcano 4
```

Suíte completa verde nesta verificação, com PostGIS efêmero via
testcontainers:

```
ok  internal/config      (cached)
ok  internal/db          71.002s
ok  internal/gvp         24.152s
ok  internal/httpapi     70.419s
ok  internal/observation 46.559s
ok  internal/source      17.952s
ok  internal/volcano     24.115s
```

Os testes que mais importam cientificamente são os de garantia, não os de
caminho feliz: `TestDataLeakage_AsOfNeverSeesFutureKnowledge` (as-of nunca vê
conhecimento futuro, verificado por sabotagem deliberada da implementação),
`TestInsert_IgnoresCallerSuppliedIngestedAt`, os testes de schema que afirmam
que `INSERT` sem `is_synthetic` falha e que fonte sem licença não pode ficar
habilitada, e o de idempotência do importador.

**Lacuna declarada:** o frontend não tem testes automatizados. `vitest` está
instalado e configurado, mas nenhuma suíte foi escrita; o CI cobre o frontend
apenas com o build. Verificação do globo até aqui foi manual, no navegador.

## 3. Tratamento de erros

- Variável de ambiente faltante ou inválida encerra o processo com código
  diferente de zero e mensagem que **nomeia a variável**.
- Banco indisponível encerra nomeando o host, após esperar até
  `DB_CONNECT_TIMEOUT`.
- Falha de migração impede o servidor de servir tráfego — não sobe degradado.
- Erro `4xx` da API nomeia o parâmetro rejeitado; erro `5xx` devolve um
  identificador de correlação e **não** expõe SQL, caminho de arquivo nem
  rastro de pilha (com teste).
- Importador: checksum divergente aborta antes de tocar no banco; registro com
  coordenada inválida é rejeitado individualmente, com log, e a importação
  segue com os demais, relatando a contagem de rejeitados.
- Parser do GVP falha explicitamente diante de formato inesperado, com teste
  sobre arquivo deliberadamente malformado.

## 4. Documentação

- `README.md` — subida, importação, API, testes, ressalva científica.
- `docs/BITEMPORAL.md` — o modelo bitemporal e como escrever uma consulta as-of
  correta, apontando o teste de data leakage como exemplo executável.
- `docs/DATA_SOURCES.md` — licença, atribuição, cadência e versão de snapshot
  por fonte, com o comando que confere documento contra banco.
- `backend/data/gvp/MANIFEST.md` — versão da base, DOI, data, checksum e as
  armadilhas de parse do arquivo real.
- `CONTRIBUTING.md`, `openspec/changes/fundacao-v0-1/{proposal,design,tasks}.md`.
- Comentários de decisão nas migrações e nos pacotes, explicando o **porquê**
  de cada invariante.

## 5. Dados de exemplo

- Snapshot real do GVP versionado em `backend/data/gvp/` — 1.214 vulcões do
  Holoceno, VOTW v5.4.0, com checksum no manifesto. Importados e servidos pela
  API nesta verificação.
- Fixtures de teste em `backend/testdata/`, explicitamente marcadas como dado
  de teste no `README.md` daquele diretório: uma amostra reduzida válida e um
  arquivo deliberadamente malformado.
- Nenhum dado sintético foi inserido no banco de desenvolvimento. A coluna
  `is_synthetic` existe e é obrigatória; a API devolve apenas dado real por
  padrão.

## 6. Validação

Camada por camada, com a validação mais forte no ponto mais baixo possível:

- **Banco:** `is_synthetic NOT NULL` sem `DEFAULT`; trigger que força
  `ingested_at = now()` em todo `INSERT`; faixa válida de latitude/longitude;
  unicidade `(source_id, source_ref)`; chaves estrangeiras para `data_sources`;
  restrição que impede habilitar fonte sem licença; restrição que impede apagar
  fonte com dados associados.
- **Domínio:** `Provenance` sem zero-value útil (esquecer de escolher é erro);
  as-of futuro rejeitado; `ingested_at` inexistente no tipo de escrita.
- **API:** faixa de latitude/longitude, janela temporal invertida, `limit`
  acima do máximo, consulta de proximidade meio-especificada, `as_of` futuro,
  valor inválido de `provenance` — todos `400` nomeando o parâmetro.

Amostra colhida na verificação, contra o serviço rodando:

```
GET /api/volcanoes                      → 404 not_found (API é versionada)
GET /api/v1/volcanoes?limit=9999        → 400 invalid_parameter (param: limit)
GET /api/v1/volcanoes?lat=-6.1          → 400 invalid_parameter (param: radius_km)
GET /health                             → 200 {"schema_version":10,"status":"ok"}
```

## 7. Logs

- Log estruturado de requisição em `internal/httpapi/middleware.go`: método,
  caminho, status, duração e identificador de correlação, com valores sensíveis
  redigidos (com teste).
- O identificador que aparece no corpo de um erro `500` é o mesmo que aparece
  no log — é assim que um erro relatado por um usuário vira uma linha
  localizável.
- Subida registra espera pelo banco, migrações aplicadas e endereço de escuta.
- Importador registra checksum verificado, contagens de inserção/atualização/
  ausência e cada registro rejeitado, individualmente.

## 8. README atualizado

Reescrito nesta tarefa (7.3) para descrever o que existe de fato: subida de
clone limpo, importação do catálogo, rotas e parâmetros reais da API, os dois
modos de rodar os testes (com e sem Go instalado), links para a documentação, e
a ressalva científica em destaque, incluindo a declaração explícita do que
**não** existe ainda.

---

## Conclusão

Os oito critérios da §88 estão satisfeitos, com uma lacuna declarada: ausência
de testes automatizados no frontend. Isso não é reclassificado como "pronto" —
fica registrado aqui e deve virar tarefa da V0.2, junto com os primeiros
conectores reais de ingestão.
