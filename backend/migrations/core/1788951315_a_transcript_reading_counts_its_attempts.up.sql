-- A transcript reading says which attempt it is on.
--
-- The AI-activity projection guards its writes lexicographically on
-- (attempt, rank): a later attempt outranks an earlier one whatever state it
-- carries, and within one attempt the ranking keeps a settled state from being
-- overwritten by a stale live one. Without a column to count them, a reading
-- re-armed after its worker died announces `queued` at the same attempt as the
-- `failed` it is recovering from — and the projection keeps the failure,
-- leaving the rail showing a dead reading while a live one runs.
--
-- attempt_at is the instant THIS attempt was enqueued, not the reading's first.
-- The projection ages a live row from it, so a reading re-armed an hour after
-- it was created would otherwise be past its lease before any worker saw it.
--
-- Both additive with defaults, so rows written before this migration read as
-- their first attempt, enqueued when they were created — which is what they
-- are. Nothing backfills, because created_at already holds that instant.

SET LOCAL lock_timeout = '3s';

ALTER TABLE transcript_read
    ADD COLUMN attempt integer NOT NULL DEFAULT 1,
    ADD COLUMN attempt_at timestamptz NOT NULL DEFAULT now();

-- A re-arm raises the attempt, so the count only ever goes up.
ALTER TABLE transcript_read
    ADD CONSTRAINT transcript_read_attempt_positive CHECK (attempt >= 1);
