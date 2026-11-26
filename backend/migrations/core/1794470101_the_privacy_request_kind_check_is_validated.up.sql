-- Validate the kind checks the migration before this one added NOT VALID.
--
-- Its own file because the runner wraps each file in one transaction, so here
-- the scan runs under SHARE UPDATE EXCLUSIVE and request writes do not queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE data_subject_request VALIDATE CONSTRAINT data_subject_request_kind_check;

ALTER TABLE privacy_request_proof VALIDATE CONSTRAINT privacy_request_proof_kind;
