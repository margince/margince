-- Bounded, like every migration that takes a lock a writer can be behind.
SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS provider_applied_field;

ALTER TABLE provider_run
  DROP COLUMN IF EXISTS applied_at;

-- REFUSE rather than destroy. Rows written under the widened vocabulary would
-- fail the narrower check, and the two ways to get past that are both worse
-- than stopping: deleting a backfill run takes its purchased claims and its
-- spend record with it (both cascade), and relabelling a no_identifiers skip as
-- not_eligible writes a fact that was never true — the provider had nothing to
-- match on, nothing forbade the purchase.
--
-- An operator who genuinely wants this rollback decides what those rows are
-- worth and clears them deliberately. A migration must not make that call.
DO $$
DECLARE offending bigint;
BEGIN
  SELECT count(*) INTO offending
    FROM provider_run
   WHERE trigger = 'automatic_backfill' OR skip_reason = 'no_identifiers';
  IF offending > 0 THEN
    RAISE EXCEPTION
      'refusing to roll back: % provider_run row(s) use the vocabulary this migration added. Deleting them would destroy purchased claims and spend history; relabelling them would record something untrue. Clear or reclassify them deliberately, then retry.', offending;
  END IF;
END $$;

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
