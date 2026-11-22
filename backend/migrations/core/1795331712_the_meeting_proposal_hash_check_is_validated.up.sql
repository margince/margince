-- The check added beside this file is validated here, under SHARE UPDATE
-- EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE meeting_proposal VALIDATE CONSTRAINT meeting_proposal_token_hash_check;
