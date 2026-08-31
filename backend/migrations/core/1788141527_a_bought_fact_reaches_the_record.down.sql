-- Bounded, like every migration that takes a lock a writer can be behind.
SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS provider_applied_field;

ALTER TABLE provider_run
  DROP COLUMN IF EXISTS applied_at;

-- The rows written under the widened vocabulary would fail the narrower check,
-- so they go before it is restored. A run that says how it was triggered is
-- worth less than a schema that matches the code reading it.
DELETE FROM provider_run WHERE trigger = 'automatic_backfill';
UPDATE provider_run SET skip_reason = 'not_eligible' WHERE skip_reason = 'no_identifiers';

ALTER TABLE provider_run
  DROP CONSTRAINT provider_run_trigger_check;

ALTER TABLE provider_run
  ADD CONSTRAINT provider_run_trigger_check CHECK (trigger IN (
    'automatic_create', 'automatic_import', 'scheduled_refresh', 'manual'));

ALTER TABLE provider_run
  DROP CONSTRAINT provider_run_skip_reason_check;

ALTER TABLE provider_run
  ADD CONSTRAINT provider_run_skip_reason_check CHECK (
    skip_reason IS NULL OR skip_reason IN (
      'budget_exhausted', 'low_balance', 'suppressed', 'not_eligible',
      'duplicate_subject_candidate', 'rate_limited', 'already_fresh'));
