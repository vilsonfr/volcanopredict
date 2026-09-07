-- +goose Up
-- Indexes backing the two access patterns the temporal storage spec
-- requires an index for: the as-of read (natural key + ingested_at DESC,
-- so DISTINCT ON picks the latest version cheaply) and range filters on
-- observed/occurred time.

CREATE INDEX observations_natural_key_ingested_idx
    ON observations (volcano_id, kind, observed_at, source_id, ingested_at DESC);
CREATE INDEX observations_observed_at_idx ON observations (observed_at);
CREATE INDEX observations_ingested_at_idx ON observations (ingested_at);

CREATE INDEX earthquakes_natural_key_ingested_idx
    ON earthquakes (external_id, source_id, ingested_at DESC);
CREATE INDEX earthquakes_occurred_at_idx ON earthquakes (occurred_at);
CREATE INDEX earthquakes_ingested_at_idx ON earthquakes (ingested_at);

-- +goose Down
DROP INDEX IF EXISTS observations_natural_key_ingested_idx;
DROP INDEX IF EXISTS observations_observed_at_idx;
DROP INDEX IF EXISTS observations_ingested_at_idx;
DROP INDEX IF EXISTS earthquakes_natural_key_ingested_idx;
DROP INDEX IF EXISTS earthquakes_occurred_at_idx;
DROP INDEX IF EXISTS earthquakes_ingested_at_idx;
