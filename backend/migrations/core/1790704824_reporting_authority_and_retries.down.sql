SET LOCAL lock_timeout = '3s';
-- Operational rollback uses the feature flag; only an unused schema can be reversed.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM report_definition) OR EXISTS(SELECT 1 FROM sales_target) OR EXISTS(SELECT 1 FROM reporting_framework_revision) THEN
  RAISE EXCEPTION 'Reporting history exists; disable analytics.performance_enabled instead of reversing migrations';
 END IF;
END $$;
DROP TRIGGER report_authority_user_change ON app_user;
DROP TRIGGER report_authority_user_delete ON app_user;
DROP TRIGGER report_authority_team_change ON team;
DROP TRIGGER report_authority_team_delete ON team;
DROP TRIGGER report_authority_role ON role;
DROP TRIGGER report_authority_role_assignment ON role_assignment;
DROP TRIGGER report_authority_team_membership ON team_membership;
DROP TRIGGER report_authority_field_mask ON field_mask;
DROP FUNCTION advance_report_projection_authority();
DROP INDEX report_execution_report_history;
ALTER TABLE report_execution DROP COLUMN retry_base_attempt;
