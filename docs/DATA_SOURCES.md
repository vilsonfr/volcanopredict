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

| Fonte | Domínio | Licença | Atribuição | Cadência de publicação | Cadência de coleta | Snapshot |
|---|---|---|---|---|---|---|
| Smithsonian Global Volcanism Program | catálogo de vulcões e erupções | `public-domain-us-govt-work-attribution-required` | Global Volcanism Program, Smithsonian Institution | atualização maior anual (normalmente até início de junho), menores a cada 6-8 semanas | manual, ao trocar o snapshot | VOTW v5.4.0, baixado em 07 Sep 2026 — `backend/data/gvp/MANIFEST.md` |
| USGS Earthquake Hazards Program | sismos | `public-domain-us-govt-work-attribution-required` | U.S. Geological Survey | contínua; eventos revisados depois de publicados | periódica, configurável | sem snapshot — ingestão por API |

## Fontes registradas e desabilitadas

Estão semeadas em `data_sources` (migrações `002`/`003`) para que o registro de
fontes exista desde o começo, mas **nenhuma tem licença determinada** e todas
estão desabilitadas pela migração `007`. Nenhum conector real foi escrito para
elas ainda — isso é a V0.2. Antes de habilitar qualquer uma é preciso ler os
termos de uso da fonte, registrar aqui licença/atribuição/cadência e escrever a
migração que preenche as colunas, como a `010` fez para o GVP.

| Fonte | Domínio | Uso pretendido | Estado |
|---|---|---|---|
| USGS Volcano Hazards Program | vulcões EUA | observações e níveis de alerta | licença não determinada — desabilitada |
| PVMBG | Indonésia | atividade vulcânica | **bloqueada** — a fonte proíbe coleta automatizada; ver seção própria abaixo |
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

## USGS Earthquake Hazards Program

Primeira fonte de ingestão contínua do projeto. O dado vem do **ANSS
Comprehensive Catalog (ComCat)**, repositório centralizado de parâmetros de
sismos produzidos pelas redes sismográficas contribuintes.

### Licenciamento

- Dado produzido ou autorado pelo USGS está em **domínio público dos EUA**:
  "USGS-authored or produced data and information are in the U.S. Public
  Domain." O reuso é livre; o que se pede é **crédito**.
- Identificador de licença gravado em `data_sources.license`:
  `public-domain-us-govt-work-attribution-required`, o mesmo do GVP, porque a
  condição é a mesma — obra de agência federal, com atribuição devida.
- Termos: <https://www.usgs.gov/information-policies-and-instructions/copyrights-and-credits>
- Ressalva: o USGS hospeda material de terceiros que **não** está em domínio
  público, e o marca como tal. Isso vale para fotos e figuras; não vale para os
  parâmetros de sismo do ComCat, que é o que este projeto ingere.

### Citação e atribuição

Citação formal do catálogo:

> U.S. Geological Survey, 2017, Advanced National Seismic System (ANSS)
> Comprehensive Catalog of Earthquake Events and Products.
> <https://doi.org/10.5066/F7MS3QZH>

Atribuição mínima em telas e respostas: *U.S. Geological Survey*, gravada em
`data_sources.attribution` e devolvida no campo `attribution` do envelope de
toda resposta que serve dado da fonte.

### Como é coletado

- **API:** FDSN Event Web Service,
  `https://earthquake.usgs.gov/fdsnws/event/1/query`.
- **Por que não os feeds GeoJSON de tempo real:** os feeds têm janela fixa e
  não expõem revisão — um evento corrigido há duas semanas não reaparece em
  `all_day`. O FDSN aceita `updatedafter`, que devolve os eventos cujo registro
  mudou desde um instante. É a pergunta que uma ingestão incremental sobre
  banco bitemporal precisa fazer.
- **Teto por requisição:** 20.000 eventos. Janela maior é percorrida em páginas.
- **Escopo:** global, sem filtro de magnitude nem de proximidade. A associação
  a vulcões é consulta geoespacial sobre o dado guardado, não critério do que
  se ingere.

### Cadência

- **Publicação:** contínua. Os feeds atualizam a cada minuto, e o catálogo
  recebe revisões a qualquer momento.
- **Revisão é a regra, não a exceção.** Um evento nasce com
  `status = automatic`, posto por processamento automático sem verificação
  humana, e pode passar a `status = reviewed` depois que uma pessoa o analisa —
  "de uma checagem rápida de validade a uma reanálise cuidadosa". Magnitudes
  preliminares, sobretudo as estimadas depressa para alerta de tsunami, são
  **substituídas** por estimativas melhores conforme chega mais dado.
- É exatamente por isso que esta é a primeira fonte: cada revisão dessas é uma
  versão nova no armazenamento bitemporal, e o que se sabia antes continua
  recuperável por consulta as-of.
- **Coleta:** periódica e configurável, ancorada na última execução
  bem-sucedida com sobreposição de segurança.

---

## PVMBG / MAGMA Indonesia — bloqueada

A fonte permanece registrada e **desabilitada**, e o bloqueio não é por
licença indeterminada: é porque a fonte **proíbe coleta automatizada**.

Evidência levantada em 07/09/2026:

- `https://magma.esdm.go.id/robots.txt` responde exatamente:

  ```
  User-agent: *
  Disallow: /
  ```

  Isto proíbe coleta automatizada do site inteiro, para qualquer cliente.
- **Não existe API pública documentada.** O endpoint que circula
  (`/v1/gunung-api/tingkat-aktivitas`) devolve **HTML**, não JSON. O projeto
  "Magma-Indonesia-API" que aparece em buscas é não-oficial e funciona
  raspando essa página — não é contrato de API, e usá-lo seria herdar a
  raspagem.
- **Não há página de termos de uso nem licença publicada.** O rodapé traz
  "Copyright 2026 © All Rights Reserved. MAGMA Indonesia".
- O próprio MAGMA credita fontes de terceiros (BMKG, USGS, GFZ, Global CMT,
  Smithsonian GVP, OpenStreetMap, ESRI), então redistribuir seu conteúdo
  envolveria também os termos dessas fontes.

Coletar dali violaria a política declarada da fonte e a regra deste projeto de
não usar fonte sem condições de uso determinadas. Não é obstáculo técnico: é
decisão de licenciamento.

**O que destrava a fonte** — qualquer um destes, e nesta ordem de preferência:

1. Permissão escrita do PVMBG/Badan Geologi autorizando acesso programático,
   com limites de taxa acordados. Contato pelo canal institucional do
   MAGMA Indonesia / Badan Geologi (ESDM).
2. Publicação, pelo PVMBG, de API oficial com termos de uso — o que tornaria o
   `robots.txt` do site irrelevante para o endpoint de dados.
3. Uma redistribuição licenciada do mesmo dado por terceiro autorizado.

Enquanto nada disso existir, o dado de nível de atividade dos vulcões
indonésios fica fora do sistema. Preencher essa lacuna com estimativa própria
seria inventar dado (§89.3) e não será feito.

Ao destravar: registrar aqui a licença, a atribuição e a cadência, e escrever a
migração que preenche as colunas e limpa o motivo do bloqueio — mesma
disciplina da `010` para o GVP.

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
