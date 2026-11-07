SET LOCAL lock_timeout = '3s';
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM privacy_notice_case WHERE source_review_note = '') THEN
        RAISE EXCEPTION 'Source selections without a note cannot be rolled back; add a note first';
    END IF;
END $$;
ALTER TABLE privacy_notice_case DROP CONSTRAINT privacy_notice_case_source_note;
ALTER TABLE privacy_notice_case ADD CONSTRAINT privacy_notice_case_source_note CHECK (
    (source_kind IS NULL) = (source_review_note IS NULL)
    AND (source_review_note IS NULL OR char_length(btrim(source_review_note)) BETWEEN 1 AND 500)
) NOT VALID;
