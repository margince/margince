SET LOCAL lock_timeout = '3s';
CREATE FUNCTION advance_report_projection_authority() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE report_projection_fence SET generation=generation+1 WHERE singleton;
 RETURN NULL;
END;
$$;
CREATE TRIGGER report_authority_user_change BEFORE UPDATE OF status,archived_at,seat_type,is_agent ON app_user
 FOR EACH STATEMENT EXECUTE FUNCTION advance_report_projection_authority();
CREATE TRIGGER report_authority_user_delete BEFORE DELETE ON app_user
 FOR EACH STATEMENT EXECUTE FUNCTION advance_report_projection_authority();
CREATE TRIGGER report_authority_team_change BEFORE UPDATE OF archived_at ON team
 FOR EACH STATEMENT EXECUTE FUNCTION advance_report_projection_authority();
CREATE TRIGGER report_authority_team_delete BEFORE DELETE ON team
 FOR EACH STATEMENT EXECUTE FUNCTION advance_report_projection_authority();
CREATE TRIGGER report_authority_role BEFORE INSERT OR UPDATE OR DELETE ON role
 FOR EACH STATEMENT EXECUTE FUNCTION advance_report_projection_authority();
CREATE TRIGGER report_authority_role_assignment BEFORE INSERT OR UPDATE OR DELETE ON role_assignment
 FOR EACH STATEMENT EXECUTE FUNCTION advance_report_projection_authority();
CREATE TRIGGER report_authority_team_membership BEFORE INSERT OR UPDATE OR DELETE ON team_membership
 FOR EACH STATEMENT EXECUTE FUNCTION advance_report_projection_authority();
CREATE TRIGGER report_authority_field_mask BEFORE INSERT OR UPDATE OR DELETE ON field_mask
 FOR EACH STATEMENT EXECUTE FUNCTION advance_report_projection_authority();
ALTER TABLE report_execution ADD COLUMN retry_base_attempt bigint NOT NULL DEFAULT 0 CHECK (retry_base_attempt >= 0 AND retry_base_attempt <= attempt);
CREATE INDEX report_execution_report_history ON report_execution(report_id,id DESC);
