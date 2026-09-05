SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_contribution DROP COLUMN stage_id;
