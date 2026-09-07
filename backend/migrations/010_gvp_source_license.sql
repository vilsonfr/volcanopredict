-- +goose Up
-- Task 5.2 (registro-de-fontes spec, design.md D5): populate license,
-- attribution, terms_url and update_cadence for the Smithsonian Global
-- Volcanism Program row seeded by 003, and enable it now that its
-- licensing has actually been reviewed (see docs/DATA_SOURCES.md for the
-- research behind these values). Every other source seeded by 003 is left
-- untouched: their license fields remain empty and 007 already forces
-- enabled = false for them, so this migration cannot leave any source
-- enabled without a license.

UPDATE data_sources
SET
    license = 'public-domain-us-govt-work-attribution-required',
    attribution = 'Global Volcanism Program, Smithsonian Institution (https://volcano.si.edu/)',
    terms_url = 'https://volcano.si.edu/gvp_termsofuse.cfm',
    update_cadence = 'annual major update (typically by early June), minor updates every 6-8 weeks',
    enabled = true
WHERE name = 'Smithsonian Global Volcanism Program';

-- +goose Down
UPDATE data_sources
SET
    license = NULL,
    attribution = NULL,
    terms_url = NULL,
    update_cadence = NULL,
    enabled = false
WHERE name = 'Smithsonian Global Volcanism Program';
