SET LOCAL lock_timeout = '3s';

ALTER TABLE meeting_proposal DROP CONSTRAINT meeting_proposal_token_hash_check;
