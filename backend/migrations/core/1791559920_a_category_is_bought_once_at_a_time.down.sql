-- Bounded, like every migration that takes a lock a writer can be behind.
SET LOCAL lock_timeout = '3s';

-- REFUSE rather than relabel. A category_in_flight skip says a live run was
-- already buying what this one asked for; calling it any other reason records
-- something that never happened. Clear or reclassify those rows deliberately.
DO $$
DECLARE offending bigint;
BEGIN
  SELECT count(*) INTO offending
    FROM provider_run WHERE skip_reason = 'category_in_flight';
  IF offending > 0 THEN
    RAISE EXCEPTION
      'refusing to roll back: % provider_run row(s) were skipped as category_in_flight, a reason the older vocabulary cannot state. Clear or reclassify them deliberately, then retry.', offending;
  END IF;
END $$;

ALTER TABLE provider_run
  DROP CONSTRAINT provider_run_skip_reason_check;

ALTER TABLE provider_run
  ADD CONSTRAINT provider_run_skip_reason_check CHECK (
    skip_reason IS NULL OR skip_reason IN (
      'budget_exhausted', 'low_balance', 'suppressed', 'not_eligible',
      'duplicate_subject_candidate', 'rate_limited', 'already_fresh',
      'no_identifiers')) NOT VALID;

DROP TRIGGER IF EXISTS trg_provider_run_release_categories ON provider_run;
DROP FUNCTION IF EXISTS provider_run_release_categories();
DROP TABLE IF EXISTS provider_run_category;
