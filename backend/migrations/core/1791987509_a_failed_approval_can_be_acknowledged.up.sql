-- The failure its decider acknowledged, held as a copy of effect_failed_at.
-- A later failure moves effect_failed_at past it, so that failure shows again.
-- Only the decider is shown a failure, so one column is one reader's mark.
--
-- effect_ran_at is when the released work last ran to the end. A run clears
-- the failure mark, and a failure that started before the run may not write one.
SET LOCAL lock_timeout = '3s';

ALTER TABLE approval
    ADD COLUMN IF NOT EXISTS acknowledged_effect_failed_at timestamptz,
    ADD COLUMN IF NOT EXISTS effect_ran_at timestamptz;
