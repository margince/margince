-- Validate the kind check the migration before this one added NOT VALID.
--
-- Its own file so the scan runs under SHARE UPDATE EXCLUSIVE. It cannot fail:
-- the new check allows every value the old one did.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_pending_counterparty VALIDATE CONSTRAINT capture_pending_counterparty_kind_check;
