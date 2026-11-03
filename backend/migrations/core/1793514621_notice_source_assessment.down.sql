SET LOCAL lock_timeout = '3s';
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM privacy_notice_case WHERE source_kind IS NOT NULL OR due_at IS NULL OR rule IN ('unassessed', 'none')) THEN
        RAISE EXCEPTION 'Source reviews and assessments retain evidence: downgrade is not supported while these records exist';
    END IF;
END $$;
ALTER TABLE privacy_notice_case
    DROP CONSTRAINT privacy_notice_case_review_shape,
    DROP CONSTRAINT privacy_notice_case_none_shape,
    DROP CONSTRAINT privacy_notice_case_due_shape,
    DROP CONSTRAINT privacy_notice_case_source_note,
    DROP CONSTRAINT privacy_notice_case_state,
    DROP CONSTRAINT privacy_notice_case_rule,
    DROP COLUMN source_kind,
    DROP COLUMN source_obtained_at,
    DROP COLUMN source_review_note,
    ALTER COLUMN due_at SET NOT NULL;
ALTER TABLE privacy_notice_case
    ADD CONSTRAINT privacy_notice_case_state CHECK (state IN (
        'open', 'assigned', 'queued', 'delivery_failed', 'completed',
        'provided_elsewhere', 'exempt_with_reason', 'blocked', 'not_required')) NOT VALID,
    ADD CONSTRAINT privacy_notice_case_rule CHECK (rule IN ('art13', 'art14')) NOT VALID;
DROP TRIGGER privacy_notice_case_owner_user_id_retirement_guard ON privacy_notice_case;
CREATE TRIGGER privacy_notice_case_owner_user_id_retirement_guard
    BEFORE INSERT OR UPDATE ON privacy_notice_case
    FOR EACH ROW WHEN (NEW.owner_user_id IS NOT NULL AND NEW.state IN (
        'open', 'assigned', 'queued', 'delivery_failed', 'blocked'))
    EXECUTE FUNCTION refuse_retired_member_work('owner_user_id');
