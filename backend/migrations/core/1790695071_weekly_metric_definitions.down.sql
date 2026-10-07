SET LOCAL lock_timeout = '3s';
-- Operational rollback uses the feature flag; only an unused schema can be reversed.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM report_definition) OR EXISTS(SELECT 1 FROM sales_target) OR EXISTS(SELECT 1 FROM reporting_framework_revision) OR EXISTS(SELECT 1 FROM weekly_review WHERE numeric_summary IS NOT NULL) OR EXISTS(SELECT 1 FROM team_weekly_review WHERE numeric_summary IS NOT NULL) THEN
  RAISE EXCEPTION 'Reporting history exists; disable analytics.performance_enabled instead of reversing migrations';
 END IF;
END $$;
ALTER TABLE weekly_review DROP COLUMN numeric_summary;
ALTER TABLE team_weekly_review DROP COLUMN numeric_summary;
