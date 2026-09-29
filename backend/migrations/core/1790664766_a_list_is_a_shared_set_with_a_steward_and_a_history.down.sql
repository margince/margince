SET LOCAL lock_timeout = '5s';

DROP TABLE IF EXISTS list_revision;
DROP TABLE IF EXISTS list_member_event;
ALTER TABLE list_member DROP COLUMN IF EXISTS note;
DROP INDEX IF EXISTS idx_list_steward;
ALTER TABLE list
    DROP CONSTRAINT IF EXISTS list_steward_id_fkey,
    DROP CONSTRAINT IF EXISTS list_sharing_check,
    DROP COLUMN IF EXISTS sharing,
    DROP COLUMN IF EXISTS steward_id,
    DROP COLUMN IF EXISTS purpose;
