-- A restriction request has no kind to fall back to, so it is refused rather
-- than rewritten: the down fails while one exists.
SET LOCAL lock_timeout = '3s';

ALTER TABLE privacy_request_proof DROP CONSTRAINT privacy_request_proof_kind;

ALTER TABLE privacy_request_proof
    ADD CONSTRAINT privacy_request_proof_kind
    CHECK (request_kind IN ('access', 'rectify', 'erasure')) NOT VALID;

ALTER TABLE data_subject_request DROP CONSTRAINT data_subject_request_kind_check;

ALTER TABLE data_subject_request
    ADD CONSTRAINT data_subject_request_kind_check
    CHECK (kind IN ('access', 'rectify', 'erasure')) NOT VALID;
