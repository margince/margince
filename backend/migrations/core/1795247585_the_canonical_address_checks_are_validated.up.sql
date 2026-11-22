-- The checks added beside this file are validated here, under SHARE UPDATE
-- EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user VALIDATE CONSTRAINT app_user_email_is_canonical;
ALTER TABLE capture_pending_counterparty VALIDATE CONSTRAINT capture_pending_counterparty_email_is_canonical;
ALTER TABLE capture_sender_override VALIDATE CONSTRAINT capture_sender_override_address_is_canonical;
ALTER TABLE communication_suppression VALIDATE CONSTRAINT communication_suppression_address_is_canonical;
