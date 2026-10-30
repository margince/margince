ALTER TABLE lead DROP COLUMN IF EXISTS arrival_owner_id;
ALTER TABLE deal_stage_history
  DROP COLUMN IF EXISTS to_position_at_change,
  DROP COLUMN IF EXISTS from_position_at_change;
