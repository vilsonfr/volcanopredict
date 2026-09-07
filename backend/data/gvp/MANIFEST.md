# Snapshot — Smithsonian Global Volcanism Program

| Campo | Valor |
|---|---|
| Arquivo | `GVP_Volcano_List_Holocene_202609071416.xls` |
| Conjunto | Volcanoes of the World — Primary Names of Holocene Volcanoes |
| Versão da base | 5.4.0 |
| Baixado em | 07 Sep 2026, 14:16 |
| Registros | 1.214 vulcões |
| Formato real | SpreadsheetML (XML do Excel 2003), apesar da extensão `.xls` |
| SHA-256 | `5f53afeafb1f3d5757b4a199c1e2d411ae81aa76edded7ad5d009bbbe8d6c7b7` |

## Citação obrigatória

> Global Volcanism Program, 2026. [Database] Volcanoes of the World (v. 5.4.0;
> 07 Sep 2026). Distributed by Smithsonian Institution, compiled by Venzke, E.
> https://doi.org/10.5479/si.GVP.VOTW5-2024.5.4

Atribuição mínima em links: *Global Volcanism Program, Smithsonian Institution*
— https://volcano.si.edu/

## Por que o arquivo está versionado aqui

A base é obra de funcionários do governo dos EUA, sem copyright doméstico; o
GVP trata o reuso como obrigação de **citação**, não como licença restritiva.
Versionar o snapshot dá reprodutibilidade (§54 da master spec): um back-test
executado hoje e daqui a um ano parte exatamente do mesmo catálogo.

Termos: https://volcano.si.edu/gvp_termsofuse.cfm

## Atenção ao parsear

**O arquivo não é XML bem formado.** Contém 254 ocorrências de `<` cru dentro
de texto, por exemplo em `Rift zone / Oceanic crust (< 15 km)`. Parsers
estritos falham com `not well-formed (invalid token)`. O parser de
`internal/gvp` escapa essas ocorrências antes de processar.

## Como atualizar

O site do GVP está atrás de desafio JavaScript da Cloudflare e retorna 403 para
clientes automatizados. O download precisa ser feito por uma pessoa, no
navegador, em https://volcano.si.edu/volcanolist_holocene.cfm. Depois:
substitua o arquivo, atualize versão, data e SHA-256 acima, e rode o importador.
