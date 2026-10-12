-- Put the column back on each table exactly as the catalog held it: jsonb,
-- nullable, no default.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company ADD COLUMN raw jsonb;
ALTER TABLE contact ADD COLUMN raw jsonb;
ALTER TABLE deal ADD COLUMN raw jsonb;
ALTER TABLE dedupe_candidate ADD COLUMN raw jsonb;
ALTER TABLE lead ADD COLUMN raw jsonb;
ALTER TABLE partner ADD COLUMN raw jsonb;
ALTER TABLE project ADD COLUMN raw jsonb;
