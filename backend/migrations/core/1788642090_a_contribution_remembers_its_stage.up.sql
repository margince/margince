-- Bound the wait for the lock below. Without it an open transaction holding a
-- conflicting lock stalls every write to this table for as long as this
-- migration is willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

-- A frozen contribution remembers which stage the deal was IN.
--
-- The pipeline-sufficiency read needs a conversion rate per stage — how many
-- deals reaching a stage were eventually won — and the only durable record of
-- where a deal stood on a given day is the snapshot that froze it. Without the
-- stage on the row, that history can only be reconstructed from the deal's
-- CURRENT stage, which answers a different question: it would credit today's
-- stage with the outcome of a deal that passed through three others.
--
-- NULLABLE, and it stays nullable. Every contribution written before this
-- migration genuinely does not know, and a backfill from the deal's present
-- stage would be that same wrong answer written down as though it were
-- recorded. The reader treats an unknown stage as one it cannot rate, which is
-- the honest handling of a row that predates the question.
--
-- NO foreign key to stage. A snapshot is a record of what was true, and a stage
-- deleted from a pipeline afterwards must not take the history with it — the
-- deal still reached that stage, whatever the pipeline looks like now.
ALTER TABLE forecast_contribution ADD COLUMN stage_id uuid;

COMMENT ON COLUMN forecast_contribution.stage_id IS
  'The stage the deal stood in when this snapshot was taken. NULL on rows written before the column existed, and on any deal whose stage could not be read; never backfilled from the deal''s current stage, which is a different fact.';
