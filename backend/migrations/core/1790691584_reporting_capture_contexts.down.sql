SET LOCAL lock_timeout = '3s';
-- Operational rollback uses the feature flag; only an unused schema can be reversed.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM report_definition) OR EXISTS(SELECT 1 FROM sales_target) OR EXISTS(SELECT 1 FROM reporting_framework_revision) OR EXISTS(SELECT 1 FROM forecast_snapshot WHERE pipeline_id IS NOT NULL OR population_fingerprint<>'') THEN
  RAISE EXCEPTION 'Reporting history exists; disable analytics.performance_enabled instead of reversing migrations';
 END IF;
END $$;
DROP INDEX uq_forecast_snapshot_daily;
DROP INDEX uq_forecast_snapshot_daily_workspace;
DROP INDEX uq_forecast_snapshot_daily_context;
DROP INDEX idx_forecast_snapshot_context;
ALTER TABLE forecast_snapshot DROP COLUMN pipeline_id, DROP COLUMN population_fingerprint;
CREATE UNIQUE INDEX uq_forecast_snapshot_daily
 ON forecast_snapshot(period_start,period_end,scope_kind,scope_id,local_day) WHERE trigger='daily';
CREATE UNIQUE INDEX uq_forecast_snapshot_daily_workspace
 ON forecast_snapshot(period_start,period_end,local_day) WHERE trigger='daily' AND scope_id IS NULL;
