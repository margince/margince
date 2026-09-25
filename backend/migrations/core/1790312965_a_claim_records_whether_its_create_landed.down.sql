SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS automation_effect_claim_unapplied;
ALTER TABLE automation_effect_claim DROP COLUMN IF EXISTS applied_at;
