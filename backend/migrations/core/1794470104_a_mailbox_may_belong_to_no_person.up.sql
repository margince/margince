-- A shared mailbox is a connection of the installation, owned by no person: its
-- user_id is NULL, an admin connected it (connected_by), and one team reads
-- what it captures (reader_team_id). Its import rows name the connection
-- instead of a seat.
--
-- The checks and the new foreign key go in NOT VALID, so these statements hold
-- ACCESS EXCLUSIVE without a scan; the next migration validates them under the
-- lighter lock. Dropping NOT NULL and adding a nullable column rewrite nothing.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection
    ALTER COLUMN user_id DROP NOT NULL,
    ADD COLUMN connected_by uuid REFERENCES app_user(id) ON DELETE SET NULL,
    ADD COLUMN reader_team_id uuid REFERENCES team(id) ON DELETE RESTRICT,
    ADD CONSTRAINT capture_connection_shared_shape CHECK (
        (user_id IS NOT NULL AND reader_team_id IS NULL)
        OR (user_id IS NULL AND reader_team_id IS NOT NULL AND account_label IS NOT NULL
            AND mail_posture <> 'classified')
    ) NOT VALID;

-- One connection per shared mailbox. The personal rule, UNIQUE (user_id,
-- provider), never meets a shared row, because NULLs are distinct.
CREATE UNIQUE INDEX capture_connection_shared_mailbox
    ON capture_connection (provider, lower(account_label)) WHERE user_id IS NULL;

ALTER TABLE capture_import
    ALTER COLUMN user_id DROP NOT NULL,
    ADD COLUMN connection_id uuid,
    ADD CONSTRAINT capture_import_connection_id_fkey
        FOREIGN KEY (connection_id) REFERENCES capture_connection(id) ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT capture_import_one_importer
        CHECK ((user_id IS NULL) <> (connection_id IS NULL)) NOT VALID;

-- A row moving to another importer changes who reads the message, as a move to
-- another seat does, so Home's cached answers are dropped for it too.
DROP TRIGGER home_read_fence_capture_import ON capture_import;
CREATE TRIGGER home_read_fence_capture_import AFTER UPDATE ON capture_import FOR EACH ROW
    WHEN (old.user_id IS DISTINCT FROM new.user_id
       OR old.activity_id IS DISTINCT FROM new.activity_id
       OR old.connection_id IS DISTINCT FROM new.connection_id)
    EXECUTE FUNCTION advance_home_read_fence();
