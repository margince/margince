-- Validates the two auth_token checks the migration before this one added, under
-- SHARE UPDATE EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE auth_token VALIDATE CONSTRAINT auth_token_purpose_check;

ALTER TABLE auth_token VALIDATE CONSTRAINT auth_token_counterpart_check;
