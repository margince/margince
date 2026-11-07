SET LOCAL lock_timeout = '3s';
ALTER TABLE privacy_notice_case
    DROP CONSTRAINT privacy_notice_case_source_evidence_shape,
    DROP COLUMN source_evidence_kind,
    DROP COLUMN source_evidence_id;
