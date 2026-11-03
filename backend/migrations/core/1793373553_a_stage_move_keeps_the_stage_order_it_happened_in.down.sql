SET LOCAL lock_timeout = '3s';


ALTER TABLE report_definition_revision DROP COLUMN IF EXISTS metric_versions;
ALTER TABLE lead DROP COLUMN IF EXISTS arrival_owner_id;
ALTER TABLE deal_stage_history
  DROP COLUMN IF EXISTS write_seq,
  DROP COLUMN IF EXISTS to_position_at_change,
  DROP COLUMN IF EXISTS from_position_at_change;
DROP SEQUENCE IF EXISTS deal_stage_history_write_seq;
