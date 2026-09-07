-- +goose Up
-- Reconciles two possible starting states so this migration applies
-- cleanly on either:
--
--   1. A brand-new volume: only PostGIS is present (docker-entrypoint-
--      initdb.d is no longer mounted), no tables exist at all.
--   2. A pre-existing development volume: the old docker-entrypoint-
--      initdb.d ran 001_init.sql/002_sources.sql directly against the
--      database, so goose's version table is empty even though the
--      tables already exist.
--
-- Every statement below is written to be safe in both cases. `volcanoes`
-- and `data_sources` keep their 001/002 shape here (goose_db_version now
-- takes over tracking); `observations` and `earthquakes` are dropped
-- because the migration plan in design.md accepts DROP/CREATE for them
-- while they are still empty in every existing environment — the last
-- time that will be true.

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

CREATE TABLE IF NOT EXISTS data_sources (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    base_url TEXT,
    category TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true
);

-- 001/002 never declared a uniqueness constraint on name, so a legacy
-- volume that already ran 002_sources.sql has no conflict target for the
-- ON CONFLICT DO NOTHING below and the seed rows would duplicate on every
-- reconciliation. A unique index is safe to add regardless of starting
-- state since 002's seed list has no duplicate names.
CREATE UNIQUE INDEX IF NOT EXISTS data_sources_name_unique ON data_sources (name);

INSERT INTO data_sources(name, base_url, category) VALUES
('Smithsonian Global Volcanism Program','https://volcano.si.edu/','volcano-history'),
('USGS Earthquake Hazards Program','https://earthquake.usgs.gov/','earthquakes'),
('USGS Volcano Hazards Program','https://www.usgs.gov/programs/VHP','volcano'),
('PVMBG','https://magma.esdm.go.id/','indonesia-volcano'),
('BMKG','https://www.bmkg.go.id/','indonesia-weather-tsunami'),
('NOAA','https://www.noaa.gov/','weather-space'),
('NASA Earthdata','https://www.earthdata.nasa.gov/','satellite'),
('Copernicus Data Space','https://dataspace.copernicus.eu/','satellite'),
('VAAC','https://www.ssd.noaa.gov/VAAC/','ash'),
('GEBCO','https://www.gebco.net/','bathymetry'),
('IRIS/EarthScope','https://www.earthscope.org/','seismology')
ON CONFLICT (name) DO NOTHING;

DROP TABLE IF EXISTS observations;
DROP TABLE IF EXISTS earthquakes;

-- +goose Down
-- No production data exists yet in any environment (see design.md,
-- Migration Plan); rollback of this phase is "discard the volume and
-- start over", which is documented rather than scripted here.
SELECT 1;
