-- Puts the checks back as the migration before this one left them: present and
-- unvalidated. Postgres cannot un-validate, so each is dropped and added NOT
-- VALID, no scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE auth_token DROP CONSTRAINT auth_token_counterpart_check;

ALTER TABLE auth_token ADD CONSTRAINT auth_token_counterpart_check
    CHECK ((purpose = 'member_merge_confirm') = (counterpart_user_id IS NOT NULL)) NOT VALID;

ALTER TABLE auth_token DROP CONSTRAINT auth_token_purpose_check;

ALTER TABLE auth_token ADD CONSTRAINT auth_token_purpose_check
    CHECK (purpose IN ('password_reset', 'email_verify', 'invite', 'member_merge_confirm')) NOT VALID;
