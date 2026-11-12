-- Put the checks back unvalidated, the way this migration found them. Postgres
-- has no ALTER that un-validates, so each is dropped and re-added NOT VALID.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_import
    DROP CONSTRAINT capture_import_one_importer,
    ADD CONSTRAINT capture_import_one_importer
        CHECK ((user_id IS NULL) <> (connection_id IS NULL)) NOT VALID;

ALTER TABLE capture_import
    DROP CONSTRAINT capture_import_connection_id_fkey,
    ADD CONSTRAINT capture_import_connection_id_fkey
        FOREIGN KEY (connection_id) REFERENCES capture_connection(id) ON DELETE RESTRICT NOT VALID;

ALTER TABLE capture_connection
    DROP CONSTRAINT capture_connection_shared_shape,
    ADD CONSTRAINT capture_connection_shared_shape CHECK (
        (user_id IS NOT NULL AND reader_team_id IS NULL)
        OR (user_id IS NULL AND reader_team_id IS NOT NULL AND account_label IS NOT NULL
            AND mail_posture <> 'classified')
    ) NOT VALID;
