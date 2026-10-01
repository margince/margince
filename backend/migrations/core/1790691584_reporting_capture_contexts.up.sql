SET LOCAL lock_timeout = '3s';
ALTER TABLE forecast_snapshot
    ADD COLUMN pipeline_id uuid REFERENCES pipeline(id),
    ADD COLUMN population_fingerprint text NOT NULL DEFAULT '';
DROP INDEX uq_forecast_snapshot_daily;
DROP INDEX uq_forecast_snapshot_daily_workspace;
CREATE UNIQUE INDEX uq_forecast_snapshot_daily
    ON forecast_snapshot(period_start,period_end,scope_kind,scope_id,local_day)
    WHERE trigger='daily' AND pipeline_id IS NULL AND population_fingerprint='';
CREATE UNIQUE INDEX uq_forecast_snapshot_daily_workspace
    ON forecast_snapshot(period_start,period_end,local_day)
    WHERE trigger='daily' AND scope_id IS NULL AND pipeline_id IS NULL AND population_fingerprint='';
CREATE UNIQUE INDEX uq_forecast_snapshot_daily_context
    ON forecast_snapshot(period_start,period_end,scope_kind,scope_id,pipeline_id,population_fingerprint,base_currency,definition_version,local_day) NULLS NOT DISTINCT
    WHERE trigger='daily' AND population_fingerprint<>'';
CREATE INDEX idx_forecast_snapshot_context
    ON forecast_snapshot(scope_kind,scope_id,pipeline_id,taken_at DESC);
