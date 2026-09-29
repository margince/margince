SET LOCAL lock_timeout = '3s';
-- Operational rollback uses the feature flag; only an unused schema can be reversed.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM report_definition) OR EXISTS(SELECT 1 FROM sales_target) OR EXISTS(SELECT 1 FROM reporting_framework_revision) THEN
  RAISE EXCEPTION 'Reporting history exists; disable analytics.performance_enabled instead of reversing migrations';
 END IF;
END $$;
ALTER TABLE report_edition DROP COLUMN expired_at;
DROP TABLE report_projection_fence;
