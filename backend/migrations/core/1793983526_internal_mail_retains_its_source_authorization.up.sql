SET lock_timeout = '3s';

ALTER TABLE activity ADD COLUMN internal_mail_authorization jsonb;
ALTER TABLE activity ADD CONSTRAINT activity_internal_mail_remains_restricted
 CHECK (internal_mail_authorization IS NULL OR (
  kind = 'email' AND audience IN ('participants', 'selected')
  AND jsonb_typeof(internal_mail_authorization) = 'object'
  AND internal_mail_authorization ?& ARRAY['connection_id', 'owner_user_id', 'provider', 'generation', 'enrolled_at', 'enrolled_by', 'policy_since']
 )) NOT VALID;
