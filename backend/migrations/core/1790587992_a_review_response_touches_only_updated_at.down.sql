CREATE OR REPLACE TRIGGER trg_activity_review_response_updated
    BEFORE UPDATE ON activity_review_response
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();
