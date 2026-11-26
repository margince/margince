-- Put the check back unvalidated, the way this migration found it. Postgres
-- has no ALTER that un-validates, so it is dropped and re-added NOT VALID.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection
    DROP CONSTRAINT capture_connection_backup_owner_is_shared,
    ADD CONSTRAINT capture_connection_backup_owner_is_shared
        CHECK (backup_owner_id IS NULL OR user_id IS NULL) NOT VALID;
