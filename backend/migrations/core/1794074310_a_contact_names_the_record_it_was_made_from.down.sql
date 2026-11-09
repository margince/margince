SET LOCAL lock_timeout = '3s';
DROP INDEX IF EXISTS idx_contact_created_from;
ALTER TABLE contact DROP COLUMN IF EXISTS created_from_activity_id;
