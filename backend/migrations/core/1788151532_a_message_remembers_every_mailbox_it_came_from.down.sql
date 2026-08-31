-- Bounded wait on the activity rewrite: dropping a column takes an ACCESS
-- EXCLUSIVE lock, and a capture sync holding a row would otherwise queue every
-- reader behind this statement for as long as the sync runs.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity DROP COLUMN IF EXISTS audience_reason;

DROP INDEX IF EXISTS capture_import_user_idx;

DROP TABLE IF EXISTS capture_import;
