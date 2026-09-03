-- What the workspace expected to close, and the record of what it expected
-- before.
--
-- A READING is derived: ask for a period, get a number computed from the deals
-- as they stand. A SNAPSHOT is frozen: the same numbers plus the per-deal rows
-- they were summed from, recorded once. Only the frozen half can answer "the
-- forecast moved — which deals moved it", because that is a question about two
-- recorded states and cannot be asked of a figure re-derived on every read.
--
-- Freezing means freezing every input, and the exchange rate is the one that
-- bites. The rate sheet is mutable: a rate corrected next week would silently
-- re-convert last week's snapshot, and the movement report would blame the
-- business for an accounting correction. So a contribution stores its own base
-- amount, rate and rate date, and every later read serves the stored integer.
SET LOCAL lock_timeout = '5s';

-- A person's assertion about what will close. Not a derivation, and not an
-- edit: calling a number never writes a deal row.
CREATE TABLE forecast_call (
    id uuid DEFAULT uuidv7() NOT NULL,
    -- The window being called, as local calendar days in the installation's
    -- reporting zone. Dates rather than instants because a period is a span of
    -- DAYS to everyone who talks about it, and storing it as a timestamptz pair
    -- would make "which quarter" depend on the reader's zone.
    period_start date NOT NULL,
    period_end date NOT NULL,
    -- Whose forecast: the whole workspace, one team, or one rep.
    scope_kind text NOT NULL,
    -- NULL exactly when the scope is the workspace, which has no id to name.
    scope_id uuid,
    amount_minor bigint NOT NULL,
    currency text NOT NULL,
    note text,
    author_id uuid NOT NULL,
    -- The call this one replaces. A call SUPERSEDES rather than overwrites, so
    -- what was believed on the day it was believed survives — which is the
    -- whole value of a call to anyone reviewing how forecasting went.
    supersedes_id uuid,
    version bigint DEFAULT 1 NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT forecast_call_pkey PRIMARY KEY (id),
    CONSTRAINT forecast_call_period_ordered CHECK (period_end >= period_start),
    CONSTRAINT forecast_call_scope_kind_check
        CHECK (scope_kind IN ('workspace', 'team', 'owner')),
    -- A workspace call names no subject and every other kind names one. Without
    -- this a team call with a null id reads as a workspace call and is counted
    -- twice.
    CONSTRAINT forecast_call_scope_id_matches_kind CHECK (
        (scope_kind = 'workspace') = (scope_id IS NULL)),
    CONSTRAINT forecast_call_amount_not_negative CHECK (amount_minor >= 0)
);

ALTER TABLE forecast_call
    ADD CONSTRAINT forecast_call_author_fkey FOREIGN KEY (author_id)
    REFERENCES app_user(id) ON DELETE RESTRICT;

-- A superseded call is not deleted: the chain is the record.
ALTER TABLE forecast_call
    ADD CONSTRAINT forecast_call_supersedes_fkey FOREIGN KEY (supersedes_id)
    REFERENCES forecast_call(id) ON DELETE RESTRICT;

CREATE INDEX idx_forecast_call_period
    ON forecast_call (period_start, period_end, scope_kind, scope_id);

-- The readings as they stood at one moment, with the rows behind them.
CREATE TABLE forecast_snapshot (
    id uuid DEFAULT uuidv7() NOT NULL,
    period_start date NOT NULL,
    period_end date NOT NULL,
    scope_kind text NOT NULL,
    scope_id uuid,
    taken_at timestamptz DEFAULT now() NOT NULL,
    -- The local calendar day taken_at fell on, in the installation's zone.
    -- Stored rather than derived because the uniqueness below is about a DAY
    -- and deriving it on read would compute it in whatever zone the reading
    -- session happens to carry.
    local_day date NOT NULL,
    trigger text NOT NULL,
    -- Which version of the metric definitions produced these numbers. A
    -- definition change is a real cause of movement and has its own bucket, so
    -- the number that moved has to say which rules it was computed under.
    definition_version text NOT NULL,
    base_currency text NOT NULL,

    -- The readings. Every one is the SUM of the stored contribution integers
    -- below, never a re-derivation: rounding happens once, per deal, at write
    -- time, which is what makes "reconciles to the cent" a property a test can
    -- hold rather than an approximation.
    won_minor bigint NOT NULL,
    evidence_minor bigint NOT NULL,
    best_case_minor bigint NOT NULL,
    open_minor bigint NOT NULL,
    weighted_minor bigint NOT NULL,

    -- What the numbers do not cover, which is the honest half of a headline.
    -- eligible_count minus priced_count is the gap a reader is owed: an
    -- unpriced deal is real pipeline and contributes zero money, and treating
    -- it as zero euros without saying so is the misreading these two prevent.
    eligible_count int NOT NULL,
    priced_count int NOT NULL,
    confirmed_date_count int NOT NULL,
    fx_missing_count int NOT NULL,

    call_id uuid,
    version bigint DEFAULT 1 NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT forecast_snapshot_pkey PRIMARY KEY (id),
    CONSTRAINT forecast_snapshot_period_ordered CHECK (period_end >= period_start),
    CONSTRAINT forecast_snapshot_scope_kind_check
        CHECK (scope_kind IN ('workspace', 'team', 'owner')),
    CONSTRAINT forecast_snapshot_scope_id_matches_kind CHECK (
        (scope_kind = 'workspace') = (scope_id IS NULL)),
    CONSTRAINT forecast_snapshot_trigger_check
        CHECK (trigger IN ('daily', 'call', 'period_close', 'recheck')),
    -- A count is a tally and a money reading is never negative pipeline.
    CONSTRAINT forecast_snapshot_counts_are_tallies CHECK (
        eligible_count >= 0 AND priced_count >= 0
        AND confirmed_date_count >= 0 AND fx_missing_count >= 0
        AND priced_count <= eligible_count
        AND confirmed_date_count <= eligible_count
        AND fx_missing_count <= eligible_count),
    CONSTRAINT forecast_snapshot_readings_not_negative CHECK (
        won_minor >= 0 AND evidence_minor >= 0 AND best_case_minor >= 0
        AND open_minor >= 0 AND weighted_minor >= 0)
);

ALTER TABLE forecast_snapshot
    ADD CONSTRAINT forecast_snapshot_call_fkey FOREIGN KEY (call_id)
    REFERENCES forecast_call(id) ON DELETE SET NULL;

-- One DAILY snapshot per period, scope and local day.
--
-- Partial on purpose. The daily job ticks repeatedly so a worker that was down
-- still backfills, and without an arbiter two ticks produce two snapshots and
-- no error. Call, period-close and recheck snapshots are deliberately
-- ADDITIONAL — a manager taking three calls in a day should get three frozen
-- states — so they sit outside it.
--
-- The same shape as uq_brief_run_user_day, whose comment says the anti-join is
-- an optimisation of the morning and the constraint is the correctness of it.
CREATE UNIQUE INDEX uq_forecast_snapshot_daily
    ON forecast_snapshot (period_start, period_end, scope_kind, scope_id, local_day)
    WHERE trigger = 'daily';

-- A partial index cannot cover a null scope_id, so the workspace scope needs
-- its own arbiter or the daily rule silently does not apply to it — the one
-- scope every installation has.
CREATE UNIQUE INDEX uq_forecast_snapshot_daily_workspace
    ON forecast_snapshot (period_start, period_end, local_day)
    WHERE trigger = 'daily' AND scope_id IS NULL;

CREATE INDEX idx_forecast_snapshot_period
    ON forecast_snapshot (period_start, period_end, scope_kind, scope_id, taken_at DESC);

-- One deal's part in one snapshot, with every input frozen.
CREATE TABLE forecast_contribution (
    id uuid DEFAULT uuidv7() NOT NULL,
    snapshot_id uuid NOT NULL,
    deal_id uuid NOT NULL,
    owner_id uuid,

    -- The deal's own money, as it stood. NULL amount is an unpriced deal:
    -- eligible, counted, contributing zero, and never treated as zero euros.
    amount_minor bigint,
    currency text,

    -- The same money in the snapshot's base currency, ALREADY ROUNDED. Every
    -- headline is the sum of these integers. A weighted total derived in SQL
    -- and one summed from these differs by up to one minor unit per deal, so
    -- the reading reads the stored value and never re-derives it.
    base_minor bigint,
    -- The rate used and the day it is from, frozen here. NULL rate means the
    -- deal was already in the base currency and no conversion happened.
    fx_rate numeric(20, 10),
    fx_date date,

    effective_close_date date,
    -- The close date is a guess the product has not confirmed. It stays out of
    -- the evidence reading, which is what "evidence" means.
    close_provisional boolean DEFAULT false NOT NULL,
    category text,
    -- Integer percent, the same type and scale as stage.win_probability, so
    -- the spellings of the weighted value in this tree cannot diverge on type.
    stage_probability int,
    -- The deal's own weighted amount, ALREADY ROUNDED, in the base currency.
    -- Stored beside base_minor because the snapshot's weighted headline is the
    -- SUM of these: a headline whose parts are not persisted cannot be
    -- reconciled against anything, which is the one promise a frozen snapshot
    -- exists to keep. NULL exactly when base_minor is — an unpriced or
    -- unconvertible deal has no weighted amount either.
    weighted_minor bigint,

    -- Which readings this deal is in. Stored rather than recomputed, because a
    -- snapshot that re-decided membership on read would not be frozen.
    in_won boolean DEFAULT false NOT NULL,
    in_evidence boolean DEFAULT false NOT NULL,
    in_best_case boolean DEFAULT false NOT NULL,
    in_open boolean DEFAULT false NOT NULL,
    -- Why a deal counted as eligible contributed no money. Named rather than
    -- implied by a null amount: "we have no price" and "we excluded it" are
    -- different facts and a reader is owed which one.
    exclusion_reason text,

    version bigint DEFAULT 1 NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT forecast_contribution_pkey PRIMARY KEY (id),
    -- An amount and its currency travel together, the same pairing
    -- deal_amount_currency_pair holds on the deal itself. Half of a money
    -- value is not a smaller money value, it is a number with no unit.
    CONSTRAINT forecast_contribution_amount_currency_pair CHECK (
        (amount_minor IS NULL) = (currency IS NULL)),
    -- A converted amount names the rate it was converted at, and a rate names
    -- the day it is from. Otherwise the freeze is not a freeze: nothing
    -- records what the number was computed with.
    CONSTRAINT forecast_contribution_rate_dated CHECK (
        (fx_rate IS NULL) = (fx_date IS NULL)),
    -- A weighted amount exists exactly when there is a base amount to weight.
    CONSTRAINT forecast_contribution_weighted_needs_a_base CHECK (
        (weighted_minor IS NULL) = (base_minor IS NULL)),
    CONSTRAINT forecast_contribution_rate_positive CHECK (
        fx_rate IS NULL OR fx_rate > 0),
    CONSTRAINT forecast_contribution_probability_is_a_percent CHECK (
        stage_probability IS NULL
        OR (stage_probability >= 0 AND stage_probability <= 100)),
    CONSTRAINT forecast_contribution_exclusion_reason_check CHECK (
        exclusion_reason IS NULL
        OR exclusion_reason IN ('unpriced', 'fx_missing', 'out_of_period', 'not_eligible'))
);

ALTER TABLE forecast_contribution
    ADD CONSTRAINT forecast_contribution_snapshot_fkey FOREIGN KEY (snapshot_id)
    REFERENCES forecast_snapshot(id) ON DELETE CASCADE;

-- One row per deal per snapshot. A second row would be counted twice into
-- every headline, and the sum would stop reconciling with no error anywhere.
ALTER TABLE forecast_contribution
    ADD CONSTRAINT uq_forecast_contribution_deal UNIQUE (snapshot_id, deal_id);

CREATE INDEX idx_forecast_contribution_snapshot
    ON forecast_contribution (snapshot_id);
CREATE INDEX idx_forecast_contribution_deal
    ON forecast_contribution (deal_id, created_at DESC);
