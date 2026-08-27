-- Dropping these returns a failed effect to being indistinguishable from one
-- that ran, which is what it was before.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS approval_effect_failed_idx;

ALTER TABLE approval DROP CONSTRAINT IF EXISTS approval_effect_failure_is_stated;

ALTER TABLE approval
    DROP COLUMN IF EXISTS effect_failed_at,
    DROP COLUMN IF EXISTS effect_failure;
