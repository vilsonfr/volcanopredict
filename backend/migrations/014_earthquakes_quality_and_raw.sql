-- +goose Up
-- Task 2.2 (qualidade-de-dados e ingestao-fontes specs, V0.2).
--
-- Três garantias, no lugar mais baixo possível:
--
-- 1. `quality_state NOT NULL` SEM DEFAULT, pela mesma disciplina de
--    `is_synthetic` (migração 004): esquecer de avaliar a qualidade falha
--    no banco, em vez de ser lido depois como se fosse `valid`. Um DEFAULT
--    aqui transformaria "ninguém avaliou" em "está bom".
-- 2. O dado cru fica ao lado do normalizado, na mesma linha e na mesma
--    transação (§18, design.md D4). Guardar em tabela separada custaria um
--    join em toda reconciliação e permitiria as duas metades divergirem.
-- 3. A versão do parser fica em cada linha. Sem ela, um bug de parse
--    corrigido depois torna impossível saber quais linhas nasceram
--    erradas, e a reprodutibilidade da §54 deixa de valer.
--
-- Sobre o preenchimento das linhas preexistentes: `ADD COLUMN ... NOT
-- NULL` sem DEFAULT falha se a tabela tiver qualquer linha, então as
-- colunas entram anuláveis, são preenchidas e só então recebem NOT NULL.
-- O que preencher é decisão científica, não detalhe: uma linha escrita
-- antes deste motor NÃO passou por verificação nenhuma, e marcá-la como
-- `valid` afirmaria que passou. Ela é marcada `suspect` com motivo que diz
-- exatamente isso — conservador e verdadeiro. Pelo mesmo raciocínio o
-- `raw` dessas linhas não recebe um `{}` que fingiria ser payload: recebe
-- um objeto que declara a ausência.

ALTER TABLE earthquakes
    ADD COLUMN quality_state TEXT,
    -- Preenchido quando o estado não é 'valid': nomeia a regra que
    -- produziu o estado, para que a razão seja rastreável até a
    -- verificação (qualidade-de-dados spec).
    ADD COLUMN quality_reason TEXT,
    ADD COLUMN parser_version TEXT,
    -- Versão do serviço/catálogo da fonte que serviu este registro, como
    -- a própria fonte a declara.
    ADD COLUMN source_version TEXT,
    ADD COLUMN raw JSONB;

UPDATE earthquakes
SET
    quality_state = 'suspect',
    quality_reason = 'unevaluated_predates_quality_engine',
    parser_version = 'unknown-pre-v0.2',
    raw = '{"_absent": "row predates raw-payload preservation; original source payload was never captured"}'::jsonb
WHERE quality_state IS NULL;

ALTER TABLE earthquakes
    ALTER COLUMN quality_state SET NOT NULL,
    ALTER COLUMN parser_version SET NOT NULL,
    ALTER COLUMN raw SET NOT NULL;

-- O conjunto de estados é da §20 da master spec. Restringir no banco
-- impede que um estado inventado entre por um caminho de escrita novo.
ALTER TABLE earthquakes
    ADD CONSTRAINT earthquakes_quality_state_known
    CHECK (quality_state IN ('valid', 'suspect', 'duplicate', 'outlier', 'corrupted', 'delayed'));

-- Estado diferente de 'valid' sem motivo seria uma marca sem explicação:
-- alguém veria 'suspect' e não teria como saber por quê.
ALTER TABLE earthquakes
    ADD CONSTRAINT earthquakes_non_valid_requires_reason
    CHECK (quality_state = 'valid' OR (quality_reason IS NOT NULL AND quality_reason <> ''));

CREATE INDEX earthquakes_quality_state_idx ON earthquakes (quality_state);

-- +goose Down
DROP INDEX IF EXISTS earthquakes_quality_state_idx;
ALTER TABLE earthquakes
    DROP CONSTRAINT IF EXISTS earthquakes_non_valid_requires_reason,
    DROP CONSTRAINT IF EXISTS earthquakes_quality_state_known,
    DROP COLUMN IF EXISTS quality_state,
    DROP COLUMN IF EXISTS quality_reason,
    DROP COLUMN IF EXISTS parser_version,
    DROP COLUMN IF EXISTS source_version,
    DROP COLUMN IF EXISTS raw;
