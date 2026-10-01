SET LOCAL lock_timeout = '3s';

ALTER TABLE lead DROP COLUMN IF EXISTS entered_at;
ALTER TABLE contact DROP COLUMN IF EXISTS entered_at;
ALTER TABLE deal DROP COLUMN IF EXISTS entered_at;
