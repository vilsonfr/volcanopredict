# Catálogo inicial de dados

| Domínio | Fonte | Uso |
|---|---|---|
| Vulcões/histórico | Smithsonian GVP | catálogo e erupções |
| Sismos | USGS | eventos sísmicos |
| Vulcões EUA | USGS VHP | observações e alertas |
| Indonésia | PVMBG | atividade vulcânica |
| Indonésia | BMKG | sismos, clima e tsunami |
| Meteorologia | NOAA | vento e atmosfera |
| Satélite | NASA Earthdata | observações remotas |
| Satélite | Copernicus | Sentinel e derivados |
| Cinzas | VAAC | avisos e trajetória de cinzas |
| Batimetria | GEBCO | simulações costeiras |
| Sismologia | EarthScope/IRIS | estações e formas de onda |

**Nota:** cada conector deve respeitar termos de uso, limites, atribuição e formato oficial da fonte. APIs/chaves devem ficar no `.env`.

## Licenciamento do Smithsonian Global Volcanism Program (GVP) — tarefa 5.1

Pesquisa de licenciamento feita para decidir se o snapshot bruto da base
*Volcanoes of the World* pode ser redistribuído dentro deste repositório
(Open Question do `design.md`, resolvida abaixo).

- A base **Volcanoes of the World** (Smithsonian Global Volcanism Program,
  GVP) é obra de funcionários do governo dos EUA, portanto sem copyright
  doméstico. O GVP trata reuso como obrigação de citação, não como licença
  restritiva.
- **Decisão:** o snapshot PODE ser redistribuído dentro do repositório,
  desde que acompanhado da citação com versão da base e data. Por isso
  `backend/data/gvp/` é o destino do arquivo bruto em si (não apenas de um
  checksum com instrução de download) — ver `MANIFEST.md`/`.json` nesse
  diretório quando um snapshot real existir.
- **Formato de citação exigido:**
  `Global Volcanism Program, <ano>. [Database] Volcanoes of the World (v. <versão>; <data>). Distributed by Smithsonian Institution, compiled by Venzke, E. <DOI>`
- **Atribuição mínima para links:** "Global Volcanism Program, Smithsonian
  Institution", com link para <https://volcano.si.edu/>.
- Fotos e imagens têm regra separada e frequentemente direitos de
  terceiros — este projeto não usa nenhuma.
- Termos de uso: <https://volcano.si.edu/gvp_termsofuse.cfm>

O identificador de licença registrado em `data_sources.license` para o GVP
é `public-domain-us-govt-work-attribution-required` (migração `010`), e o
texto de atribuição e a URL dos termos são gravados nas colunas
`attribution` e `terms_url` da mesma linha.

### Situação da fonte em 2026 e tentativa de obtenção do snapshot (tarefa 5.3)

O GVP descontinuou busca e API enquanto reconstrói o site ao longo de
2026. O que resta publicamente é o download em massa da *Holocene Volcano
List*. Foi tentado, nesta execução da tarefa 5.3:

- `curl -L -A "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36" https://volcano.si.edu/volcanolist_holocene.cfm` → **HTTP 403**, corpo é uma página de bloqueio da Cloudflare ("Attention Required! | Cloudflare"), não o site do GVP.
- Mesma tentativa contra `https://volcano.si.edu/projects/vaac-data/` → **HTTP 403**, mesmo bloqueio da Cloudflare.
- Tentativa via ferramenta de busca web (`WebFetch`) contra a primeira URL → também **HTTP 403 Forbidden**.

Como o bloqueio ocorre antes de qualquer HTML da página real ser servido,
não há link de download a inspecionar; a Cloudflare está rejeitando a
requisição no nível de bot-detection, não apenas por causa do parser da
página.

**Conclusão:** o snapshot real não pôde ser obtido nesta execução. Nenhum
arquivo foi gravado em `backend/data/gvp/`. O parser e o importador
(`internal/gvp`) foram implementados e testados contra uma fixture
sintética criada à mão em `backend/testdata/gvp_holocene_fixture.csv`,
explicitamente marcada como dado de teste — **não é o catálogo real e não
substitui a importação real**. As tarefas 5.3, 5.7 e 5.8 permanecem
desmarcadas em `tasks.md` até que alguém consiga baixar o snapshot real
(por exemplo de uma rede/IP não bloqueado pela Cloudflare, ou por um
mirror oficial) e rode a importação de fato.

