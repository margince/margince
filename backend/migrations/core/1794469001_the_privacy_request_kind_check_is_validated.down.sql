-- Put the checks back unvalidated, the way this migration found them. Postgres
-- has no ALTER that un-validates, so each is dropped and re-added NOT VALID.
SET LOCAL lock_timeout = '3s';

ALTER TABLE data_subject_request
    DROP CONSTRAINT data_subject_request_kind_check,
    ADD CONSTRAINT data_subject_request_kind_check
        CHECK (kind IN ('access', 'rectify', 'erasure', 'restriction')) NOT VALID;

ALTER TABLE privacy_request_proof
    DROP CONSTRAINT privacy_request_proof_kind,
    ADD CONSTRAINT privacy_request_proof_kind
        CHECK (request_kind IN ('access', 'rectify', 'erasure', 'restriction')) NOT VALID;
