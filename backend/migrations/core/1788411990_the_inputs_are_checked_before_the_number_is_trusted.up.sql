-- What last night's pass asked of the pipeline, and what it found.
--
-- A forecast is only as good as its inputs, and the failures are mundane: a
-- close date that went by, an amount that disagrees with the offer that was
-- sent, a deal nobody has heard from in ninety days. None is a bug in the
-- arithmetic; every one makes the total wrong.
--
-- The run row is as important as the findings. A pass that could not read the
-- mailbox has checked less of the pipeline than one that could, and a reader
-- told only "3 exceptions" cannot tell those apart.
SET LOCAL lock_timeout = '5s';

-- One nightly pass.
CREATE TABLE assurance_run (
    id uuid DEFAULT uuidv7() NOT NULL,
    as_of timestamptz NOT NULL,
    -- Which version of each rule produced this run's findings. A rule change
    -- is a real cause of a finding appearing or vanishing, and a reader
    -- comparing two nights needs to tell that from the business changing.
    rule_versions jsonb DEFAULT '{}'::jsonb NOT NULL,

    -- How much there was to check. Counted in the scan loop, one per record
    -- actually evaluated — not re-derived from a count query, which would make
    -- the assertion x == x and leave a loop that broke early looking complete.
    eligible_deals int DEFAULT 0 NOT NULL,
    eligible_signals int DEFAULT 0 NOT NULL,

    -- running while it works; complete when every rule ran; incomplete when an
    -- upstream was unavailable. The worker NEVER refuses to start: refusing
    -- would produce no run at all in exactly the case this exists to report,
    -- which is a broken connector.
    status text DEFAULT 'running' NOT NULL,
    -- What the run entitles a reader to conclude. `checks_incomplete` is not a
    -- worse `needs_review`: one says the pipeline has problems, the other says
    -- we could not look, and telling a manager the first when the second is
    -- true is the failure this column prevents.
    readiness text,
    digest text,

    version bigint DEFAULT 1 NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT assurance_run_pkey PRIMARY KEY (id),
    CONSTRAINT assurance_run_status_check
        CHECK (status IN ('running', 'complete', 'incomplete')),
    CONSTRAINT assurance_run_readiness_check
        CHECK (readiness IS NULL OR readiness IN
            ('ready', 'ready_with_exceptions', 'needs_review', 'checks_incomplete')),
    -- A finished run has said what it entitles a reader to conclude. One still
    -- running has not, and a readiness on it would be read as a verdict.
    CONSTRAINT assurance_run_readiness_matches_status CHECK (
        (status = 'running') = (readiness IS NULL)),
    CONSTRAINT assurance_run_counts_are_tallies CHECK (
        eligible_deals >= 0 AND eligible_signals >= 0)
);

CREATE INDEX idx_assurance_run_as_of ON assurance_run (as_of DESC);

-- Which sources a run reached, and how far.
--
-- Its own table rather than columns on the run, because the source list grows:
-- an installation that connects a contract store gains a source to check, and
-- a run predating that connection must not read as having checked it.
CREATE TABLE assurance_source_coverage (
    id uuid DEFAULT uuidv7() NOT NULL,
    run_id uuid NOT NULL,
    source text NOT NULL,
    state text NOT NULL,
    -- How current the source was when it was read. NULL where nothing was
    -- read: a permission-limited source has no "checked through" date, and a
    -- stale timestamp there would read as "checked up to yesterday" when
    -- nothing was checked at all.
    checked_through timestamptz,

    version bigint DEFAULT 1 NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT assurance_source_coverage_pkey PRIMARY KEY (id),
    CONSTRAINT assurance_source_coverage_source_check CHECK (
        source IN ('mail', 'calendar', 'documents', 'contracts', 'offers', 'incumbent')),
    CONSTRAINT assurance_source_coverage_state_check CHECK (
        state IN ('checked', 'stale', 'unavailable', 'permission_limited')),
    -- Only a source that was actually read has a date. The other three states
    -- mean nothing was read, and a date on them is a claim about coverage that
    -- did not happen.
    CONSTRAINT assurance_source_coverage_date_matches_state CHECK (
        (state = 'checked') OR checked_through IS NULL)
);

ALTER TABLE assurance_source_coverage
    ADD CONSTRAINT assurance_source_coverage_run_fkey FOREIGN KEY (run_id)
    REFERENCES assurance_run(id) ON DELETE CASCADE;

-- One source per run: two rows for `mail` would be two answers to one question.
ALTER TABLE assurance_source_coverage
    ADD CONSTRAINT uq_assurance_source_coverage_run_source UNIQUE (run_id, source);

-- One thing worth a person's attention.
CREATE TABLE assurance_exception (
    id uuid DEFAULT uuidv7() NOT NULL,

    -- The stable identity: exception type plus the record it is about, and
    -- never the value observed. Keyed on the value, a close date that moved
    -- twice would be three exceptions and somebody would resolve the same
    -- finding three times.
    logical_key text NOT NULL,
    type text NOT NULL,
    subject_kind text NOT NULL,
    subject_id uuid NOT NULL,

    -- What the record CLAIMS and what was OBSERVED, as structured values only —
    -- a date, a minor-unit amount, an id. Never a snippet lifted from an
    -- activity body: a frozen copy of a protected sentence outlives the
    -- protection on its source, which is how narrowing after birth stops being
    -- privacy. A gate holds the key set per type.
    claim jsonb DEFAULT '{}'::jsonb NOT NULL,
    observed jsonb DEFAULT '{}'::jsonb NOT NULL,
    -- Pointers to what was read, re-checked at hydration on every surface.
    -- Discoverability is not permission to show a body.
    evidence_refs jsonb DEFAULT '[]'::jsonb NOT NULL,

    severity text NOT NULL,
    -- How much money the finding puts in question, where it can be said. NULL
    -- for a finding about a record with no amount — different from zero, which
    -- would claim nothing is at stake.
    affected_minor bigint,
    currency text,
    owner_id uuid,

    first_seen_at timestamptz DEFAULT now() NOT NULL,
    last_seen_at timestamptz DEFAULT now() NOT NULL,
    status text DEFAULT 'open' NOT NULL,
    -- When an upstream event invalidated what this row observed. A finding
    -- about a deal whose amount just changed is stale until the next scan, and
    -- showing it as current would send somebody to check a number that already
    -- moved.
    input_changed_at timestamptz,

    version bigint DEFAULT 1 NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT assurance_exception_pkey PRIMARY KEY (id),
    CONSTRAINT assurance_exception_status_check
        CHECK (status IN ('open', 'resolved', 'expired')),
    CONSTRAINT assurance_exception_severity_check
        CHECK (severity IN ('low', 'medium', 'high')),
    CONSTRAINT assurance_exception_subject_kind_check
        CHECK (subject_kind IN ('deal', 'signal', 'offer', 'contract')),
    -- An amount and its currency travel together, the same pairing the deal
    -- itself holds. Half a money value is a number with no unit.
    CONSTRAINT assurance_exception_amount_currency_pair CHECK (
        (affected_minor IS NULL) = (currency IS NULL)),
    CONSTRAINT assurance_exception_seen_ordered CHECK (last_seen_at >= first_seen_at)
);

-- The identity, enforced. Re-running a scan finds the same condition and must
-- update the row it already minted rather than adding a second — otherwise a
-- manager resolves the same finding every morning.
ALTER TABLE assurance_exception
    ADD CONSTRAINT uq_assurance_exception_logical_key UNIQUE (logical_key);

CREATE INDEX idx_assurance_exception_open
    ON assurance_exception (status, severity, last_seen_at DESC)
    WHERE status = 'open';
CREATE INDEX idx_assurance_exception_subject
    ON assurance_exception (subject_kind, subject_id);
CREATE INDEX idx_assurance_exception_owner
    ON assurance_exception (owner_id) WHERE owner_id IS NOT NULL;

-- What somebody decided about a finding.
CREATE TABLE assurance_resolution (
    id uuid DEFAULT uuidv7() NOT NULL,
    exception_id uuid NOT NULL,
    outcome text NOT NULL,
    reason text,
    evidence_ref text,
    -- When to raise it again. A resolution is not always permanent: "not now"
    -- is a real answer and it comes back.
    remind_at timestamptz,
    -- When the resolution stops holding. A capped expiry rather than forever:
    -- "the value is correct" is true of a value, and values change.
    expires_at timestamptz,
    actor_id uuid NOT NULL,

    version bigint DEFAULT 1 NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT assurance_resolution_pkey PRIMARY KEY (id),
    CONSTRAINT assurance_resolution_outcome_check CHECK (outcome IN (
        'value_correct', 'record_corrected', 'not_material',
        'condition_cleared', 'deferred', 'not_mine')),
    CONSTRAINT assurance_resolution_expiry_after_creation CHECK (
        expires_at IS NULL OR expires_at > created_at)
);

ALTER TABLE assurance_resolution
    ADD CONSTRAINT assurance_resolution_exception_fkey FOREIGN KEY (exception_id)
    REFERENCES assurance_exception(id) ON DELETE CASCADE;

ALTER TABLE assurance_resolution
    ADD CONSTRAINT assurance_resolution_actor_fkey FOREIGN KEY (actor_id)
    REFERENCES app_user(id) ON DELETE RESTRICT;

CREATE INDEX idx_assurance_resolution_exception
    ON assurance_resolution (exception_id, created_at DESC);
