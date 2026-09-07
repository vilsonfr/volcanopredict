-- +goose Up
-- Recreates `observations` in bitemporal, append-only form (design.md D1,
-- D2, D6). Natural key: (volcano_id, kind, observed_at, source_id).
-- Uniqueness is on (natural key, ingested_at), never on the natural key
-- alone, so a source revision inserts a new row instead of colliding.

CREATE TABLE observations (
    id BIGSERIAL PRIMARY KEY,
    volcano_id BIGINT NOT NULL REFERENCES volcanoes(id),
    kind TEXT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    -- Assigned by the database, never by the caller (D1, D6). No
    -- application code should set this column explicitly.
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    value JSONB NOT NULL,
    -- No DEFAULT, deliberately: an INSERT that omits this column fails at
    -- the database, so "forgot to mark provenance" can never be silently
    -- read back as "real data" (design.md D6).
    is_synthetic BOOLEAN NOT NULL,
    source_id BIGINT NOT NULL REFERENCES data_sources(id),
    CONSTRAINT observations_natural_key UNIQUE (volcano_id, kind, observed_at, source_id, ingested_at)
);

-- +goose Down
DROP TABLE IF EXISTS observations;
