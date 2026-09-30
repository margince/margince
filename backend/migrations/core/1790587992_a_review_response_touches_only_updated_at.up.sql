-- activity_review_response has no version column, so its touch trigger calls
-- the function that writes updated_at alone. set_updated_at_bump_version also
-- writes NEW.version, which failed every UPDATE of the table.
CREATE OR REPLACE TRIGGER trg_activity_review_response_updated
    BEFORE UPDATE ON activity_review_response
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
