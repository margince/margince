-- A shared mailbox may name one backup owner: the seat that holds work from
-- its mail when the sender's contact and company have no owner who may read
-- it. Only a shared row names one. The check goes in NOT VALID, so this holds
-- ACCESS EXCLUSIVE without a scan; the next migration validates it. A nullable
-- column rewrites nothing.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection
    ADD COLUMN backup_owner_id uuid REFERENCES app_user(id) ON DELETE SET NULL,
    ADD CONSTRAINT capture_connection_backup_owner_is_shared CHECK (backup_owner_id IS NULL OR user_id IS NULL) NOT VALID;
