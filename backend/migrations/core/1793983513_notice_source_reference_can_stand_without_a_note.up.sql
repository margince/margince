SET LOCAL lock_timeout = '3s';
ALTER TABLE privacy_notice_case DROP CONSTRAINT privacy_notice_case_source_note;
ALTER TABLE privacy_notice_case ADD CONSTRAINT privacy_notice_case_source_note CHECK (
    (source_kind IS NULL) = (source_review_note IS NULL)
    AND (source_review_note IS NULL OR (
        char_length(btrim(source_review_note)) <= 500
        AND (char_length(btrim(source_review_note)) > 0 OR source_evidence_id IS NOT NULL)
    ))
) NOT VALID;
