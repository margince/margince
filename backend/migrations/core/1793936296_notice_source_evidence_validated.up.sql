SET LOCAL lock_timeout = '3s';
ALTER TABLE privacy_notice_case VALIDATE CONSTRAINT privacy_notice_case_source_evidence_shape;
