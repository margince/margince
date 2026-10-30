-- Two facts a cohort needs and the rows did not keep.
--
-- A stage move records both stages' positions as they stood at the move.
-- Stage order is configuration, and reordering stages today must not turn an
-- old advance into a regression. Rows written before this have NULL here and
-- are reported as partial history; no source records the old positions.
--
-- A lead records who owned it when it arrived, the instant its response clock
-- runs from. owner_id is the owner now, and a reassignment must not move a
-- lead's arrival to its new owner. No foreign key, as with
-- deal_stage_history.owner_id_at_change: a removed seat keeps its history.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_stage_history
  ADD COLUMN from_position_at_change integer,
  ADD COLUMN to_position_at_change integer;

ALTER TABLE lead ADD COLUMN arrival_owner_id uuid;

COMMENT ON COLUMN deal_stage_history.from_position_at_change IS
  'The position of from_stage_id in its pipeline at the move; null on a first entry and on rows written before positions were kept.';
COMMENT ON COLUMN deal_stage_history.to_position_at_change IS
  'The position of to_stage_id in its pipeline at the move; null on rows written before positions were kept.';
COMMENT ON COLUMN lead.arrival_owner_id IS
  'The owner at the instant COALESCE(routed_at, created_at); null when the lead arrived unowned or before this was recorded.';
