-- +goose Up
-- Adds licensing/attribution/cadence fields and an audit trail to
-- data_sources (registro-de-fontes spec). A source SHALL NOT be enabled
-- while its license is empty, so every row seeded by 003 (which predates
-- license review) is disabled here — deliberately conservative. Task 5.2
-- is the one that later populates license/attribution per source and
-- re-enables the ones cleared for use; until then nothing should be
-- ingestible.

ALTER TABLE data_sources
    ADD COLUMN license TEXT,
    ADD COLUMN attribution TEXT,
    ADD COLUMN terms_url TEXT,
    ADD COLUMN update_cadence TEXT,
    ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- Existing rows predate license review; enabling with no license is
-- exactly what the new CHECK below forbids, so disable them up front
-- instead of leaving the migration unable to complete.
UPDATE data_sources SET enabled = false WHERE license IS NULL OR license = '';

ALTER TABLE data_sources
    ADD CONSTRAINT data_sources_enabled_requires_license
    CHECK (NOT enabled OR (license IS NOT NULL AND license <> ''));

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER data_sources_set_updated_at
    BEFORE UPDATE ON data_sources
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS data_sources_set_updated_at ON data_sources;
DROP FUNCTION IF EXISTS set_updated_at();
ALTER TABLE data_sources
    DROP CONSTRAINT IF EXISTS data_sources_enabled_requires_license,
    DROP COLUMN IF EXISTS license,
    DROP COLUMN IF EXISTS attribution,
    DROP COLUMN IF EXISTS terms_url,
    DROP COLUMN IF EXISTS update_cadence,
    DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS updated_at;
