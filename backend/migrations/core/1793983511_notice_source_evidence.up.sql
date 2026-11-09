SET LOCAL lock_timeout = '3s';
ALTER TABLE privacy_notice_case
    ADD COLUMN source_evidence_kind text,
    ADD COLUMN source_evidence_id uuid,
    ADD CONSTRAINT privacy_notice_case_source_evidence_shape CHECK (
        (source_evidence_kind IS NULL AND source_evidence_id IS NULL)
        OR (source_evidence_kind IN ('acquisition', 'activity') AND source_evidence_kind IS NOT NULL AND source_evidence_id IS NOT NULL)
    ) NOT VALID;
