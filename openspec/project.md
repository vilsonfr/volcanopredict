# Contexto do Projeto — VolcanoPredict

## Propósito

Plataforma científica aberta para monitoramento, análise e detecção de anomalias em atividade vulcânica.

O sistema **não** promete prever erupções. A finalidade primária é reconstruir o histórico e o estado quase em tempo real dos vulcões, e identificar sinais estatisticamente anormais com incerteza explícita. Capacidade preditiva só é desenvolvida depois, sobre back-testing temporal rigoroso.

A especificação de longo prazo vive em `VOLCANO_PREDICTION_MASTER_SPEC.md` (127 seções). Ela é a fonte de verdade para *o que* o produto deve virar; as changes do OpenSpec são a fonte de verdade para *o que está sendo construído agora*.

## Stack

- **Backend:** Go 1.24, `net/http` da stdlib, `pgx/v5` para Postgres
- **Banco:** PostgreSQL 16 + PostGIS 3.4
- **Frontend:** React 19 + TypeScript + Vite 7 (CesiumJS previsto para V0.3)
- **Containers:** Docker Compose
- **Testes:** `go test`, Vitest

## Convenções

- Código, identificadores e comentários em inglês. Documentação, specs e mensagens ao usuário em pt-BR.
- Migrações são arquivos SQL numerados e imutáveis em `backend/migrations/`. Nunca editar uma migração já aplicada; criar a próxima.
- Todo endpoint HTTP vive sob `/api/v1/`.
- Segredos e chaves de API só via `.env`; nunca commitados.

## Restrições científicas invioláveis

Derivadas das §4, §28 e §89 da master spec. Valem para toda change:

1. **Dado real primeiro.** Dado sintético só existe se explicitamente marcado como tal no schema e na resposta da API. Nunca misturar silenciosamente com observação real.
2. **Bitemporalidade.** Toda observação registra *quando aconteceu* e *quando o sistema soube*. Sem isso não há back-testing honesto.
3. **Sem informação futura.** Análise histórica só enxerga o que estava disponível naquele instante. Nada de normalização com estatística do futuro.
4. **Incerteza explícita.** Score sem qualidade de dado e contagem de sensores é score inválido.
5. **Toda fonte externa é registrada** em `data_sources` com licença e termos de uso.
6. **Score experimental nunca é alerta oficial.** A interface deve dizer isso.

## Ordem de implementação

A §87 da master spec fixa a ordem obrigatória. Não pular etapas, não começar por ML. Posição atual: passos 1–5 (repositório, arquitetura, banco, catálogo, registro de fontes) em andamento na V0.1.
