SET lock_timeout = '3s';

DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM capture_backfill WHERE internal_mail_authorization IS NOT NULL) THEN
  RAISE EXCEPTION 'Remove authorized internal history runs before rolling back';
 END IF;
END $$;
ALTER TABLE capture_backfill DROP CONSTRAINT capture_backfill_internal_mail_authorization;
ALTER TABLE capture_backfill DROP COLUMN internal_mail_authorization;
