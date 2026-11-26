-- Fails while a shared mailbox or its import rows exist: a seat-less row has no
-- personal shape to return to.
SET LOCAL lock_timeout = '3s';

DROP TRIGGER home_read_fence_capture_import ON capture_import;
CREATE TRIGGER home_read_fence_capture_import AFTER UPDATE ON capture_import FOR EACH ROW
    WHEN (old.user_id IS DISTINCT FROM new.user_id OR old.activity_id IS DISTINCT FROM new.activity_id)
    EXECUTE FUNCTION advance_home_read_fence();

ALTER TABLE capture_import
    DROP CONSTRAINT capture_import_one_importer,
    DROP CONSTRAINT capture_import_connection_id_fkey,
    DROP COLUMN connection_id,
    ALTER COLUMN user_id SET NOT NULL;

DROP INDEX capture_connection_shared_mailbox;

ALTER TABLE capture_connection
    DROP CONSTRAINT capture_connection_shared_shape,
    DROP COLUMN reader_team_id,
    DROP COLUMN connected_by,
    ALTER COLUMN user_id SET NOT NULL;
