# O modelo bitemporal e como escrever uma consulta as-of correta

Este é o documento a ler antes de escrever qualquer consulta que envolva
tempo neste projeto. A regra curta: **nenhuma análise histórica pode enxergar
conhecimento que o sistema ainda não tinha na data analisada.** Tudo abaixo
existe para tornar essa regra difícil de violar por acidente.

## Os dois tempos

Toda linha de dado observacional carrega dois instantes, e eles significam
coisas diferentes:

| Coluna | Significa | Quem atribui |
|---|---|---|
| `observed_at` (ou `occurred_at`, em `earthquakes`) | quando o **fenômeno** aconteceu no mundo | a fonte externa |
| `ingested_at` | quando **este sistema ficou sabendo** do fenômeno | o banco de dados |

A distinção importa porque as duas linhas do tempo não andam juntas. Uma fonte
pode publicar hoje um sismo de três dias atrás; pode corrigir amanhã a
magnitude que publicou ontem. Se um back-test perguntar "o que eu sabia em
15/03?" e a resposta incluir a correção que só chegou em 17/03, o modelo está
sendo treinado com informação do futuro — *data leakage* — e o resultado do
back-test é fantasia.

## Append-only: revisão é linha nova, nunca `UPDATE`

A unicidade em `observations` é sobre **(chave natural, `ingested_at`)**, nunca
sobre a chave natural sozinha:

```sql
CONSTRAINT observations_natural_key
    UNIQUE (volcano_id, kind, observed_at, source_id, ingested_at)
```

Consequência direta: quando a fonte revisa um dado, o ingestor **insere outra
linha** com o mesmo `(volcano_id, kind, observed_at, source_id)` e um
`ingested_at` novo. As duas versões coexistem para sempre. Nada é sobrescrito,
nada é apagado (§89.12 da master spec). Em `earthquakes` a chave natural é
`(external_id, source_id)` — o identificador da própria fonte.

"Estado corrente" deixa de ser uma linha e passa a ser um resultado de
consulta: a versão de maior `ingested_at` por chave natural.

## `ingested_at` é do banco, não do chamador

Três camadas guardam isso, de fora para dentro:

1. `DEFAULT now()` na coluna (migração `004`) — cobre quem omite a coluna.
2. Um trigger `BEFORE INSERT` que faz `NEW.ingested_at = now()` incondicionalmente
   (migração `009`) — cobre quem **fornece** um valor, inclusive um script
   avulso rodando `psql` fora da aplicação. O `DEFAULT` sozinho não faria nada
   nesse caso.
3. O tipo `observation.NewObservation` em Go não tem campo `IngestedAt`, e o
   `INSERT` de `observation.Insert` não lista a coluna. Não há por onde um
   chamador passar o valor.

A camada 2 é a garantia de verdade; as outras duas existem para que o erro seja
impossível de escrever, não apenas ineficaz.

Do mesmo modo, `is_synthetic BOOLEAN NOT NULL` **sem `DEFAULT`**: um `INSERT`
que esquece a proveniência falha no banco, em vez de ser lido depois como dado
real (§89.7 — não misturar dados reais e simulados).

## Como escrever uma consulta as-of

**Não escreva.** Chame `internal/observation`.

Esse pacote é o único lugar do código autorizado a escrever predicado temporal
sobre `observed_at`/`ingested_at` (design.md D7). Qualquer outro pacote que
precise de "o que eu sabia no instante T" chama `observation.List` — é isso que
mantém a garantia auditável num ponto só, em vez de espalhada por dez `WHERE`
diferentes que envelhecem de formas diferentes.

```go
asOf := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
obs, err := observation.List(ctx, pool, observation.Query{
    VolcanoID:  &volcanoID,
    Kind:       &kind,
    AsOf:       &asOf,                       // nil = conhecimento corrente
    Provenance: observation.ProvenanceRealOnly,
})
```

O que `List` garante, e por quê:

- **`WHERE ingested_at <= $asOf`** — o predicado que impede o vazamento. É a
  primeira cláusula da consulta, antes de qualquer filtro de negócio.
- **`DISTINCT ON (volcano_id, kind, observed_at, source_id) ... ORDER BY ...
  ingested_at DESC`** — escolhe a versão mais recente *que já existia em `asOf`*.
  Sem isso, uma chave natural revisada três vezes voltaria três vezes.
- **`AsOf` no futuro é rejeitado** com `ErrFutureAsOf`, em vez de ser tratado
  silenciosamente como "agora". Perguntar pelo futuro é erro de quem pergunta.
- **`AsOf` nil significa "agora"**, o que é literalmente a mesma consulta com
  `asOf = time.Now()`. Não existe um caminho de código "sem filtro temporal"
  que alguém possa acabar usando por engano.
- **`Provenance` não tem valor-padrão útil**: o zero-value é
  `ProvenanceUnspecified` e é *rejeitado*. Quem esquece de escolher recebe erro,
  não uma mistura implícita de dado real com sintético.
- **Tudo em UTC**, sempre, na entrada e na saída.

Os índices que sustentam isso estão na migração `006`: um por
`(chave natural, ingested_at DESC)` para o `DISTINCT ON`, e um por
`observed_at` para os filtros de janela.

## O exemplo executável: o teste de data leakage

`TestDataLeakage_AsOfNeverSeesFutureKnowledge`, em
[backend/internal/observation/observation_test.go:334](../backend/internal/observation/observation_test.go#L334),
é a especificação executável desta página. Leia-o junto com este documento —
ele mostra o caso difícil melhor do que a prosa.

O que ele monta: cinco inserções intercaladas com cinco *checkpoints*. Uma
delas é uma **observação de chegada tardia** — o fenômeno aconteceu em
`base+2h`, antes do fenômeno inserido logo acima dela, mas o sistema só ficou
sabendo depois do checkpoint 2. É exatamente o padrão que quebra a implementação
ingênua que filtra só por `observed_at`.

O que ele afirma:

1. Para cada checkpoint, **nenhuma** linha retornada tem `ingested_at` posterior
   ao instante consultado. Essa é a asserção de vazamento.
2. E — crucialmente — a linha de chegada tardia está **ausente** no checkpoint 2
   e **presente** no checkpoint 3. Sem essa segunda parte, a asserção (1)
   passaria de forma vazia caso a consulta simplesmente não retornasse nada.

A propriedade de sabotagem: **se o predicado `ingested_at <= asOf` for removido
ou enfraquecido em `observation.List`, este teste tem de ficar vermelho.** Foi
verificado sabotando a implementação de propósito e vendo o teste falhar. Ao
mexer em qualquer coisa neste caminho, refaça essa sabotagem — um teste temporal
que passa com a garantia quebrada é pior que nenhum teste.

Ele é um teste de integração contra PostGIS real: roda em `go test ./...` e é
pulado em `go test -short ./...`.

## Erros a não cometer

- Filtrar por `observed_at` achando que se está fazendo uma consulta histórica.
  `observed_at` é o fenômeno; a janela de conhecimento é `ingested_at`.
- Escrever `SELECT ... FROM observations WHERE ...` fora de
  `internal/observation`.
- `UPDATE` numa linha de `observations` para "corrigir" um valor. A correção é
  uma linha nova.
- `DELETE` de qualquer coisa histórica.
- Aceitar `ingested_at` de um payload externo.
- Agregar sem dizer a proveniência.
