-- +goose Up
-- Adds catalog provenance to volcanoes (design.md D5): source_id + the
-- source's own volcano number (source_ref), unique per source, plus
-- absent_from_source_at so a volcano that disappears from a future
-- snapshot is marked rather than deleted (project.md constraint #1,
-- master spec §89 item 12).
--
-- Latitude/longitude validation cannot be expressed as a CHECK on the
-- existing `location GEOGRAPHY(POINT,4326)` column: PostGIS silently
-- coerces out-of-range coordinates into the valid range at construction
-- time (e.g. ST_GeogFromText('POINT(105.4 91)') stores latitude 89 with
-- only a NOTICE, never an error), so a CHECK reading the geography back
-- would evaluate the already-clamped value and never fire. This was
-- verified by hand against postgis/postgis:16-3.4 — not something
-- design.md anticipated. Explicit latitude/longitude columns, validated
-- before the geography is derived, are the only way to actually reject
-- bad input. `location` is kept in sync by trigger and remains the
-- column spatial queries use.

ALTER TABLE volcanoes
    ADD COLUMN source_id BIGINT REFERENCES data_sources(id),
    ADD COLUMN source_ref TEXT,
    ADD COLUMN absent_from_source_at TIMESTAMPTZ,
    ADD COLUMN latitude DOUBLE PRECISION,
    ADD COLUMN longitude DOUBLE PRECISION;

ALTER TABLE volcanoes
    ADD CONSTRAINT volcanoes_source_ref_unique UNIQUE (source_id, source_ref);

ALTER TABLE volcanoes
    ADD CONSTRAINT volcanoes_latitude_valid_range CHECK (latitude BETWEEN -90 AND 90),
    ADD CONSTRAINT volcanoes_longitude_valid_range CHECK (longitude BETWEEN -180 AND 180),
    ADD CONSTRAINT volcanoes_lat_lon_together CHECK ((latitude IS NULL) = (longitude IS NULL));

-- Backfill from the existing geography column so rows written before this
-- migration keep coordinates queryable through the new columns too.
UPDATE volcanoes
SET latitude = ST_Y(location::geometry), longitude = ST_X(location::geometry)
WHERE location IS NOT NULL;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION volcanoes_sync_location()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.latitude IS NOT NULL AND NEW.longitude IS NOT NULL THEN
        NEW.location = ST_SetSRID(ST_MakePoint(NEW.longitude, NEW.latitude), 4326)::geography;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER volcanoes_sync_location_trigger
    BEFORE INSERT OR UPDATE OF latitude, longitude ON volcanoes
    FOR EACH ROW
    EXECUTE FUNCTION volcanoes_sync_location();

-- +goose Down
DROP TRIGGER IF EXISTS volcanoes_sync_location_trigger ON volcanoes;
DROP FUNCTION IF EXISTS volcanoes_sync_location();
ALTER TABLE volcanoes
    DROP CONSTRAINT IF EXISTS volcanoes_lat_lon_together,
    DROP CONSTRAINT IF EXISTS volcanoes_longitude_valid_range,
    DROP CONSTRAINT IF EXISTS volcanoes_latitude_valid_range,
    DROP CONSTRAINT IF EXISTS volcanoes_source_ref_unique,
    DROP COLUMN IF EXISTS source_id,
    DROP COLUMN IF EXISTS source_ref,
    DROP COLUMN IF EXISTS absent_from_source_at,
    DROP COLUMN IF EXISTS latitude,
    DROP COLUMN IF EXISTS longitude;
