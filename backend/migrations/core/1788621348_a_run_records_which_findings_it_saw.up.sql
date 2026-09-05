-- Which findings a run actually observed.
--
-- assurance_run records how MUCH a night checked — eligible deals, eligible
-- signals — and assurance_exception records what is open now. Neither answers
-- "which run last confirmed this exception", and that is the question a manager
-- doubting a finding asks first: is this still true, or is it a leftover from a
-- night before the deal moved?
--
-- Today the only per-night trace is the cleared count on the run's audit row,
-- which says how many findings closed and names none of them.
--
-- WHY MEMBERSHIP AND NOT A COLUMN ON THE EXCEPTION. A `last_seen_run_id` would
-- answer the same question for the newest run and lose every earlier one, so
-- "was this finding present three nights running, or did it come and go" would
-- stay unanswerable. The exception's own identity is stable across nights by
-- design (logical_key); its PRESENCE is what varies, and presence is a fact
-- about the pair.

SET LOCAL lock_timeout = '3s';

CREATE TABLE assurance_run_finding (
    run_id uuid NOT NULL REFERENCES assurance_run(id) ON DELETE CASCADE,
    exception_id uuid NOT NULL REFERENCES assurance_exception(id) ON DELETE CASCADE,
    -- Observed at the run's own as_of, not at insert time: a run is a reading
    -- taken at an instant, and the row belongs to that instant rather than to
    -- whenever the transaction happened to commit.
    observed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    -- One row per (run, finding). A run that walked a deal twice — a retry
    -- inside one transaction, a rule that fires on two subjects sharing a
    -- logical key — records the observation once.
    CONSTRAINT assurance_run_finding_pkey PRIMARY KEY (run_id, exception_id)
);

-- The question this table exists for: which run last confirmed THIS exception.
CREATE INDEX assurance_run_finding_by_exception
  ON assurance_run_finding (exception_id, observed_at DESC);

-- "Everything one run saw" needs no index of its own: run_id leads the primary
-- key, so that lookup already has one.
