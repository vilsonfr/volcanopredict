-- +goose Up
-- Task 2.1 (sismos spec, V0.2). A tabela guardava o mínimo — onde, quando,
-- quão forte. A §9 da master spec exige que cada integração registre
-- também unidade, precisão, qualidade e status; sem isso não dá para
-- distinguir uma solução automática preliminar de uma revisada por uma
-- pessoa, e as duas acabariam pesando igual em qualquer análise.
--
-- Todas as colunas são opcionais porque a fonte de fato as omite: um
-- evento pequeno pode não ter magnitude, e nem toda rede reporta gap
-- azimutal. Ausência aqui é informação, não falha — por isso NULL, e não
-- um zero que mentiria.

ALTER TABLE earthquakes
    -- 'automatic' (posto por processamento automático, sem verificação
    -- humana) ou 'reviewed' (analisado por uma pessoa). A promoção de um
    -- para o outro é a revisão mais comum do ComCat.
    ADD COLUMN source_status TEXT,
    -- 'mww', 'mb', 'ml'... Magnitudes de tipos diferentes não são
    -- comparáveis entre si sem conversão explícita (§21), então o tipo
    -- viaja junto do número.
    ADD COLUMN magnitude_type TEXT,
    -- Indicadores de qualidade da solução, como a fonte os publica.
    -- Descrevem a confiança na localização, não o valor em si.
    ADD COLUMN rms DOUBLE PRECISION,
    ADD COLUMN azimuthal_gap DOUBLE PRECISION,
    ADD COLUMN station_count INTEGER,
    ADD COLUMN min_distance_deg DOUBLE PRECISION,
    -- O instante em que a FONTE diz ter atualizado o evento. Distinto do
    -- nosso ingested_at, e não confiável como chave de deduplicação
    -- (design.md D3) — mas registrável, e útil para diagnóstico.
    ADD COLUMN source_updated_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE earthquakes
    DROP COLUMN IF EXISTS source_status,
    DROP COLUMN IF EXISTS magnitude_type,
    DROP COLUMN IF EXISTS rms,
    DROP COLUMN IF EXISTS azimuthal_gap,
    DROP COLUMN IF EXISTS station_count,
    DROP COLUMN IF EXISTS min_distance_deg,
    DROP COLUMN IF EXISTS source_updated_at;
