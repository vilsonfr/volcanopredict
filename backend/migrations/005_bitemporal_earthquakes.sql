-- +goose Up
-- Recreates `earthquakes` in bitemporal, append-only form with the same
-- discipline as observations (design.md D1, D2, D6). The natural key here
-- is the source's own identifier for the event, scoped by source_id
-- because different sources may reuse identifier schemes.

CREATE TABLE earthquakes (
    id BIGSERIAL PRIMARY KEY,
    external_id TEXT NOT NULL,
    source_id BIGINT NOT NULL REFERENCES data_sources(id),
    occurred_at TIMESTAMPTZ NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    location GEOGRAPHY(POINT, 4326) NOT NULL,
    magnitude DOUBLE PRECISION,
    depth_km DOUBLE PRECISION,
    is_synthetic BOOLEAN NOT NULL,
    CONSTRAINT earthquakes_natural_key UNIQUE (external_id, source_id, ingested_at)
);

-- +goose Down
DROP TABLE IF EXISTS earthquakes;
