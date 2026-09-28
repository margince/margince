-- A claim records whether the create it guards actually landed.
--
-- The claim commits in its own transaction and the domain create runs after it,
-- so a worker dying between the two left a claim with no record behind it. The
-- next firing folded as "deduplicated" against a write that never happened, and
-- the task was lost with nothing to repair it.
--
-- applied_at closes that. NULL means claimed-but-not-yet-applied: either a
-- firing is in flight right now, or one died holding it. A sibling tells the
-- two apart by age, so a claim stranded by a crash becomes reclaimable while a
-- live one is still respected.
--
-- Nullable and with no backfill: every claim already in the table guards a
-- create that completed, because the old code only ever inserted before a
-- create it then ran to completion in the same call. Stamping them would claim
-- a precision about history this column does not have, and leaving them NULL
-- would make each look stranded and re-appliable. The partial index below
-- reads only the unapplied ones, which for existing rows is none.
-- Bounded: an open transaction holding a conflicting lock would otherwise stall
-- every write to this table for as long as this migration is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE automation_effect_claim
	ADD COLUMN applied_at timestamptz;

UPDATE automation_effect_claim SET applied_at = created_at WHERE applied_at IS NULL;

-- Only the unapplied rows are ever scanned by age, and they are the rare ones.
CREATE INDEX automation_effect_claim_unapplied
	ON automation_effect_claim (created_at)
	WHERE applied_at IS NULL;
