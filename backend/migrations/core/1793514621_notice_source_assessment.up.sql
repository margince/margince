SET LOCAL lock_timeout = '3s';
ALTER TABLE privacy_notice_case
    ALTER COLUMN due_at DROP NOT NULL,
    ADD COLUMN source_kind text,
    ADD COLUMN source_obtained_at timestamptz,
    ADD COLUMN source_review_note text,
    DROP CONSTRAINT privacy_notice_case_state,
    DROP CONSTRAINT privacy_notice_case_rule;
ALTER TABLE privacy_notice_case
    ADD CONSTRAINT privacy_notice_case_state CHECK (state IN (
        'source_review', 'open', 'assigned', 'queued', 'delivery_failed',
        'completed', 'provided_elsewhere', 'exempt_with_reason', 'blocked', 'not_required')) NOT VALID,
    ADD CONSTRAINT privacy_notice_case_rule CHECK (rule IN ('unassessed', 'none', 'art13', 'art14')) NOT VALID,
    ADD CONSTRAINT privacy_notice_case_review_shape CHECK (
        (state = 'source_review') = (rule = 'unassessed') AND (state <> 'source_review' OR (source_kind IS NULL AND due_at IS NULL AND cardinality(allowed_routes) = 0))) NOT VALID,
    ADD CONSTRAINT privacy_notice_case_none_shape CHECK (rule <> 'none' OR (due_at IS NULL AND completed_at IS NOT NULL AND cardinality(allowed_routes) = 0)) NOT VALID,
    ADD CONSTRAINT privacy_notice_case_due_shape CHECK (
        state NOT IN ('open', 'assigned', 'queued', 'delivery_failed', 'blocked') OR due_at IS NOT NULL) NOT VALID,
    ADD CONSTRAINT privacy_notice_case_source_note CHECK (
        (source_kind IS NULL) = (source_review_note IS NULL)
        AND (source_review_note IS NULL OR char_length(btrim(source_review_note)) BETWEEN 1 AND 500)) NOT VALID;
DROP TRIGGER privacy_notice_case_owner_user_id_retirement_guard ON privacy_notice_case;
CREATE TRIGGER privacy_notice_case_owner_user_id_retirement_guard
    BEFORE INSERT OR UPDATE ON privacy_notice_case
    FOR EACH ROW WHEN (NEW.owner_user_id IS NOT NULL AND NEW.state IN (
        'open', 'assigned', 'queued', 'delivery_failed', 'blocked', 'source_review'))
    EXECUTE FUNCTION refuse_retired_member_work('owner_user_id');
