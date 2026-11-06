SET lock_timeout = '3s';

ALTER TABLE capture_backfill ADD COLUMN internal_mail_authorization jsonb;
ALTER TABLE capture_backfill ADD CONSTRAINT capture_backfill_internal_mail_authorization
 CHECK (internal_mail_authorization IS NULL OR (
  jsonb_typeof(internal_mail_authorization) = 'object'
  AND internal_mail_authorization ?& ARRAY['admission', 'after', 'before', 'authorized_by']
 )) NOT VALID;
