-- Bounded, like every migration that takes a lock a writer can be behind.
SET LOCAL lock_timeout = '3s';

-- A guard on admission and nothing else: one row per live run and category it
-- asks for, so a second run that shares a category with a live run on the same
-- contact is refused, whatever the rest of its set is. The live-run fingerprint
-- index only catches a repeat of the WHOLE set. What a run bought stays on
-- provider_run.requested_categories; a row here exists exactly while its run is
-- live, so its existence is its liveness and the index needs no predicate.
--
-- contact_id and provider are copied for the unique index, and the composite
-- key binds them to the run: a merge that moves the run moves its claims, and
-- the copy can never name a contact the run does not.
CREATE TABLE provider_run_category (
  run_id     uuid NOT NULL,
  contact_id uuid NOT NULL,
  provider   text NOT NULL,
  category   text NOT NULL,
  PRIMARY KEY (run_id, contact_id, provider, category),
  CONSTRAINT provider_run_category_run_fkey
    FOREIGN KEY (run_id, contact_id, provider)
    REFERENCES provider_run (id, contact_id, provider)
    ON UPDATE CASCADE ON DELETE CASCADE,
  CONSTRAINT provider_run_category_one_live_buyer UNIQUE (contact_id, provider, category)
);

-- The release lives on the table rather than in each writer, because the state
-- is written from many places and a release list kept across them misses the
-- next one. BEFORE, not AFTER: the subject scrub sets contact_id to NULL on a
-- run that may still be live, and the claims must be gone before the key
-- cascade would try to copy that NULL into a NOT NULL column.
CREATE FUNCTION provider_run_release_categories() RETURNS trigger
  LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM provider_run_category WHERE run_id = OLD.id;
  RETURN NEW;
END $$;

CREATE TRIGGER trg_provider_run_release_categories
  BEFORE UPDATE OF state, contact_id ON provider_run
  FOR EACH ROW
  WHEN (NEW.state NOT IN ('queued', 'submitting', 'in_progress', 'submission_unknown')
        OR NEW.contact_id IS NULL)
  EXECUTE FUNCTION provider_run_release_categories();

ALTER TABLE provider_run
  DROP CONSTRAINT provider_run_skip_reason_check;

ALTER TABLE provider_run
  ADD CONSTRAINT provider_run_skip_reason_check CHECK (
    skip_reason IS NULL OR skip_reason IN (
      'budget_exhausted', 'low_balance', 'suppressed', 'not_eligible',
      'duplicate_subject_candidate', 'rate_limited', 'already_fresh',
      'no_identifiers', 'category_in_flight')) NOT VALID;
