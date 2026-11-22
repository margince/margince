-- A proposal's token_hash is a SHA-256 in hex, or the mark an erasure leaves:
-- the erased prefix and the activity id. The check is added unvalidated.
SET LOCAL lock_timeout = '3s';

ALTER TABLE meeting_proposal ADD CONSTRAINT meeting_proposal_token_hash_check
    CHECK (token_hash ~ '^[0-9a-f]{64}$' OR token_hash ~ '^erased:[0-9a-f-]{36}$') NOT VALID;
