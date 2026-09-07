# Fontes de dados

Toda fonte usada pelo projeto é registrada na tabela `data_sources` e tem uma
entrada neste documento. A regra que liga os dois é dura, e o banco a impõe:
a restrição `data_sources_enabled_requires_license` (migração `007`) recusa
qualquer fonte habilitada sem `license` preenchida. Fonte cuja licença ainda
não foi lida por uma pessoa fica **desabilitada** e não é consultada por
nenhum conector.

Para conferir que este documento e o banco não divergiram:

```bash
docker compose exec db psql -U volcano -d volcanopredict -c \
  "SELECT name, enabled, license, update_cadence FROM data_sources ORDER BY name;"
```

Toda linha com `enabled = true` precisa ter uma seção detalhada abaixo.

## Fontes habilitadas

| Fonte | Domínio | Licença | Atribuição | Cadência | Snapshot |
|---|---|---|---|---|---|
| Smithsonian Global Volcanism Program | catálogo de vulcões e erupções | `public-domain-us-govt-work-attribution-required` | Global Volcanism Program, Smithsonian Institution | atualização maior anual (normalmente até início de junho), menores a cada 6-8 semanas | VOTW v5.4.0, baixado em 07 Sep 2026 — `backend/data/gvp/MANIFEST.md` |

## Fontes registradas e desabilitadas

Estão semeadas em `data_sources` (migrações `002`/`003`) para que o registro de
fontes exista desde o começo, mas **nenhuma tem licença determinada** e todas
estão desabilitadas pela migração `007`. Nenhum conector real foi escrito para
elas ainda — isso é a V0.2. Antes de habilitar qualquer uma é preciso ler os
termos de uso da fonte, registrar aqui licença/atribuição/cadência e escrever a
migração que preenche as colunas, como a `010` fez para o GVP.

| Fonte | Domínio | Uso pretendido | Estado |
|---|---|---|---|
| USGS Earthquake Hazards Program | sismos | eventos sísmicos | licença não determinada — desabilitada |
| USGS Volcano Hazards Program | vulcões EUA | observações e níveis de alerta | licença não determinada — desabilitada |
| PVMBG | Indonésia | atividade vulcânica | licença não determinada — desabilitada |
| BMKG | Indonésia | sismos, clima e tsunami | licença não determinada — desabilitada |
| NOAA | meteorologia | vento e atmosfera | licença não determinada — desabilitada |
| NASA Earthdata | satélite | observações remotas | licença não determinada — desabilitada |
| Copernicus Data Space | satélite | Sentinel e derivados | licença não determinada — desabilitada |
| VAAC | cinzas | avisos e trajetória de cinzas | licença não determinada — desabilitada |
| GEBCO | batimetria | simulações costeiras | licença não determinada — desabilitada |
| IRIS/EarthScope | sismologia | estações e formas de onda | licença não determinada — desabilitada |

Chaves e tokens de API, quando alguma fonte exigir, ficam no `.env` e nunca no
repositório.

---

## Smithsonian Global Volcanism Program (GVP)

### Licenciamento — decisão da tarefa 5.1

Pesquisa feita para decidir se o snapshot bruto da base *Volcanoes of the
World* pode ser redistribuído dentro deste repositório (Open Question do
`design.md`, resolvida abaixo).

- A base **Volcanoes of the World** é obra de funcionários do governo dos EUA,
  portanto sem copyright doméstico. O GVP trata reuso como obrigação de
  citação, não como licença restritiva.
- **Decisão:** o snapshot PODE ser redistribuído dentro do repositório, desde
  que acompanhado da citação com versão da base e data. Por isso o arquivo
  bruto está versionado em `backend/data/gvp/`, e não apenas referenciado por
  checksum e instrução de download.
- Fotos e imagens do GVP têm regra separada e frequentemente direitos de
  terceiros — este projeto não usa nenhuma.
- Termos de uso: <https://volcano.si.edu/gvp_termsofuse.cfm>

### Citação obrigatória

> Global Volcanism Program, 2026. [Database] Volcanoes of the World (v. 5.4.0;
> 07 Sep 2026). Distributed by Smithsonian Institution, compiled by Venzke, E.
> <https://doi.org/10.5479/si.GVP.VOTW5-2024.5.4>

Atribuição mínima em links: *Global Volcanism Program, Smithsonian Institution*,
com link para <https://volcano.si.edu/>.

O identificador de licença gravado em `data_sources.license` é
`public-domain-us-govt-work-attribution-required` (migração `010`); atribuição
e URL dos termos ficam nas colunas `attribution` e `terms_url` da mesma linha,
e a API devolve esse texto no campo `attribution` do envelope de toda resposta
que serve dado da fonte.

### Snapshot versionado

| Campo | Valor |
|---|---|
| Arquivo | `backend/data/gvp/GVP_Volcano_List_Holocene_202609071416.xls` |
| Conjunto | Volcanoes of the World — Primary Names of Holocene Volcanoes |
| Versão da base | 5.4.0 |
| Baixado em | 07 Sep 2026, 14:16 |
| Registros | 1.214 vulcões |
| Formato real | SpreadsheetML (XML do Excel 2003), apesar da extensão `.xls` |
| SHA-256 | `5f53afeafb1f3d5757b4a199c1e2d411ae81aa76edded7ad5d009bbbe8d6c7b7` |

O importador verifica esse SHA-256 contra o `MANIFEST.md` antes de tocar no
banco: snapshot trocado sem atualizar o manifesto aborta a importação.

### Cadência e como atualizar

O GVP publica uma atualização maior por ano, normalmente até o início de junho,
e atualizações menores a cada 6-8 semanas.

O site está atrás de desafio JavaScript da Cloudflare e devolve `403` para
clientes automatizados — o download precisa ser feito por uma pessoa, no
navegador, em <https://volcano.si.edu/volcanolist_holocene.cfm>. Depois:
substitua o arquivo, atualize versão, data, contagem e SHA-256 no
`MANIFEST.md` e nesta seção, e rode `import-catalog`. O importador nunca apaga:
vulcão que sumiu da fonte é marcado em `absent_from_source_at`.
