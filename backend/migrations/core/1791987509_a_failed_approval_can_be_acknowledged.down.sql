SET LOCAL lock_timeout = '3s';

ALTER TABLE approval
    DROP COLUMN IF EXISTS acknowledged_effect_failed_at,
    DROP COLUMN IF EXISTS effect_ran_at;
