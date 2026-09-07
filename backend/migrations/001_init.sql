CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE IF NOT EXISTS volcanoes (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    country TEXT,
    location GEOGRAPHY(POINT, 4326),
    elevation_m DOUBLE PRECISION,
    status TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS observations (
    id BIGSERIAL PRIMARY KEY,
    volcano_id BIGINT REFERENCES volcanoes(id),
    observed_at TIMESTAMPTZ NOT NULL,
    kind TEXT NOT NULL,
    value JSONB NOT NULL,
    source TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS earthquakes (
    id TEXT PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL,
    location GEOGRAPHY(POINT, 4326) NOT NULL,
    magnitude DOUBLE PRECISION,
    depth_km DOUBLE PRECISION,
    source TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS data_sources (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    base_url TEXT,
    category TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true
);

CREATE INDEX IF NOT EXISTS earthquakes_time_idx ON earthquakes (occurred_at);
CREATE INDEX IF NOT EXISTS observations_time_idx ON observations (observed_at);
