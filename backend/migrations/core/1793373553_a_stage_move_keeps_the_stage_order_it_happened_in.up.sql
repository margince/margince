-- Facts a cohort needs and the rows did not keep.
--
-- A stage move records both stages' positions as they stood at the move, and a
-- write sequence. Stage order is configuration, and a reorder must not turn an
-- old advance into a regression. Two moves of one deal can share a
-- millisecond, and uuidv7 does not order them, so the sequence does. Rows
-- written before this have NULL here and are reported as partial history.
--
-- A lead records who owned it when it arrived, the instant its response clock
-- runs from. No foreign key, as with deal_stage_history.owner_id_at_change: a
-- removed seat keeps its history.
--
-- A saved report revision records each metric's definition version, so a
-- revision saved under one definition is never read under another.
SET LOCAL lock_timeout = '3s';

CREATE SEQUENCE deal_stage_history_write_seq;

ALTER TABLE deal_stage_history
  ADD COLUMN from_position_at_change integer,
  ADD COLUMN to_position_at_change integer,
  ADD COLUMN write_seq bigint;
-- Set after the ADD, so existing rows keep NULL and the table is not rewritten.
ALTER TABLE deal_stage_history ALTER COLUMN write_seq SET DEFAULT nextval('deal_stage_history_write_seq');
ALTER SEQUENCE deal_stage_history_write_seq OWNED BY deal_stage_history.write_seq;

-- The default privileges cover tables only, so the app role needs USAGE to call
-- nextval(). Conditional because a throwaway test database has no app role.
DO $$
BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'margince_app') THEN
    GRANT USAGE, SELECT ON SEQUENCE deal_stage_history_write_seq TO margince_app;
  END IF;
END $$;

ALTER TABLE lead ADD COLUMN arrival_owner_id uuid;

ALTER TABLE report_definition_revision ADD COLUMN metric_versions jsonb;

COMMENT ON COLUMN deal_stage_history.from_position_at_change IS
  'The position of from_stage_id in its pipeline at the move; null on a first entry and on rows written before positions were kept.';
COMMENT ON COLUMN deal_stage_history.to_position_at_change IS
  'The position of to_stage_id in its pipeline at the move; null on rows written before positions were kept.';
COMMENT ON COLUMN deal_stage_history.write_seq IS
  'The order the moves were written in, for moves that share an instant; null on rows written before it was kept.';
COMMENT ON COLUMN lead.arrival_owner_id IS
  'The owner at the instant COALESCE(routed_at, created_at); null when the lead arrived unowned or before this was recorded.';
COMMENT ON COLUMN report_definition_revision.metric_versions IS
  'Each selected metric''s definition version when the revision was saved; null on revisions saved before it was kept.';
