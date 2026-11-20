-- The check added beside this file is validated here, under SHARE UPDATE
-- EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE confirm_token VALIDATE CONSTRAINT confirm_token_lead_is_only_told;
