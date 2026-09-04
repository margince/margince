-- Bounded, because ALTER TABLE takes an ACCESS EXCLUSIVE lock on a table every
-- staged approval is written to: an open transaction holding a conflicting
-- lock would otherwise stall every write to it for as long as this migration
-- is willing to queue.
SET LOCAL lock_timeout = '3s';

-- A tag merge is a TWO-row operation with a ONE-row precondition.
--
-- approval pins target_entity_*, which for a merge is the word being retired.
-- The word it folds INTO is named in the card a human reads and pinned by
-- nothing, so it can be renamed while the card is pending and the merge still
-- runs as though it had not been. A human approves folding into "SkewTarget";
-- it folds into whatever that row is called by the time the agent retries.
--
-- The retired side has been protected all along, which is what made the
-- asymmetry easy to miss.
--
-- Nullable, and NULL for every approval that pins one row: an operation whose
-- meaning rests on a single record is the ordinary case and stays exactly as
-- it was. The re-check at redemption short-circuits on NULL the same way the
-- primary pin already does.
ALTER TABLE approval
  ADD COLUMN co_target_entity_type text,
  ADD COLUMN co_target_entity_id uuid,
  ADD COLUMN co_target_version bigint;

-- All three or none. A type and an id with no version is a pin that cannot be
-- re-checked, and a version with nothing to read it from is worse — the
-- redemption would either skip the check or look up a row it cannot name, and
-- both read as "this was verified" to anyone auditing the trail.
ALTER TABLE approval
  ADD CONSTRAINT approval_co_target_whole CHECK (
    (co_target_entity_type IS NULL AND co_target_entity_id IS NULL AND co_target_version IS NULL)
    OR
    (co_target_entity_type IS NOT NULL AND co_target_entity_id IS NOT NULL AND co_target_version IS NOT NULL)
  );
