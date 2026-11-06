SET lock_timeout = '3s';

ALTER TABLE capture_backfill VALIDATE CONSTRAINT capture_backfill_internal_mail_authorization;
