-- +goose Up
-- Belt-and-suspenders for design.md D1/D6 and the top-level quality rule
-- that ingested_at is "atribuído pelo banco/sistema e ignorado se o
-- chamador mandar": the DEFAULT now() on ingested_at only helps when the
-- caller omits the column. If a caller (or a stray script bypassing the
-- application layer entirely) explicitly supplies a value, DEFAULT does
-- nothing to stop it. These triggers force ingested_at to now() on every
-- INSERT regardless of what was supplied, making the invariant a database
-- guarantee rather than an application-layer convention.

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION force_ingested_at_now()
RETURNS TRIGGER AS $$
BEGIN
    NEW.ingested_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER observations_force_ingested_at
    BEFORE INSERT ON observations
    FOR EACH ROW
    EXECUTE FUNCTION force_ingested_at_now();

CREATE TRIGGER earthquakes_force_ingested_at
    BEFORE INSERT ON earthquakes
    FOR EACH ROW
    EXECUTE FUNCTION force_ingested_at_now();

-- +goose Down
DROP TRIGGER IF EXISTS observations_force_ingested_at ON observations;
DROP TRIGGER IF EXISTS earthquakes_force_ingested_at ON earthquakes;
DROP FUNCTION IF EXISTS force_ingested_at_now();
