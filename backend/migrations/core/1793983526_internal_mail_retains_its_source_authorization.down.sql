SET lock_timeout = '3s';

DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM activity WHERE internal_mail_authorization IS NOT NULL) THEN
  RAISE EXCEPTION 'Remove internal operational mail through the privacy lifecycle before rolling back';
 END IF;
END $$;
ALTER TABLE activity DROP CONSTRAINT activity_internal_mail_remains_restricted;
ALTER TABLE activity DROP COLUMN internal_mail_authorization;
