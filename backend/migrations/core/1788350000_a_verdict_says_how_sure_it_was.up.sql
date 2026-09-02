-- A sender verdict records how sure it was and which model answered.
--
-- Neither survived the decision before this. The engine compared the
-- confidence against a floor and dropped it, so a 0.71 answer and a 0.99 one
-- were indistinguishable afterwards: an operator asking why a department was
-- filed as a person could see the answer and never how close it came to being
-- refused. The served model matters for the same reason — this lane runs on
-- whatever local model the deployment bound, and a wrong answer is evidence
-- about that model only if the model is named.
--
-- Both are nullable, because every row already on the ledger was decided
-- without them and no backfill can invent what was never measured. NULL here
-- means "decided before this was recorded", which is the honest answer.
-- Bounded, because this locks a table capture writes to on every captured
-- message: an open transaction holding a conflicting lock would otherwise
-- stall every capture for as long as this migration is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_pending_counterparty
    ADD COLUMN confidence numeric,
    ADD COLUMN served_model text;

-- A confidence outside [0,1] is not a confidence. The check is here rather
-- than only in Go because the column is read back by operators and by the
-- review queue, and a value nothing could have meant would be believed.
ALTER TABLE capture_pending_counterparty
    ADD CONSTRAINT capture_pending_counterparty_confidence_range
    CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1));
