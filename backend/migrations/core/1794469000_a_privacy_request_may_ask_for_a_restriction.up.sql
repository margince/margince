-- A privacy request may ask for a restriction of processing (GDPR Art. 18),
-- and its proof names that kind.
--
-- Both kind checks go in NOT VALID, so the swap holds ACCESS EXCLUSIVE without
-- a scan; the next migration validates them under the lighter lock.
SET LOCAL lock_timeout = '3s';

ALTER TABLE data_subject_request DROP CONSTRAINT data_subject_request_kind_check;

ALTER TABLE data_subject_request
    ADD CONSTRAINT data_subject_request_kind_check
    CHECK (kind IN ('access', 'rectify', 'erasure', 'restriction')) NOT VALID;

ALTER TABLE privacy_request_proof DROP CONSTRAINT privacy_request_proof_kind;

ALTER TABLE privacy_request_proof
    ADD CONSTRAINT privacy_request_proof_kind
    CHECK (request_kind IN ('access', 'rectify', 'erasure', 'restriction')) NOT VALID;
