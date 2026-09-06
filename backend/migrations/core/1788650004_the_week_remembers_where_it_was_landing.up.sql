-- What the week's forecast was, frozen beside the review that reports it.
--
-- The review already freezes its deals and their labels, for the reason its own
-- migration states: a retrospective is a record of what a week WAS, and a row
-- that quietly changes because somebody cleaned up a deal is a record nobody
-- can trust. A landing is the same kind of fact and needs the same treatment.
--
-- Snapshot IDS ARE NOT ENOUGH. forecast_snapshot is subject to retention, so a
-- review holding only a pointer reads as a blank outlook the day the snapshot
-- ages out — and a blank is indistinguishable from a week nobody measured. So
-- the FIGURES are copied here beside the ids: the ids say which snapshots this
-- was read from while they still exist, and the copies are what the panel
-- draws forever.
SET LOCAL lock_timeout = '3s';

-- One row per horizon per review: a week is read over its own week, its month
-- and its fiscal quarter, and the three are different landings.
CREATE TABLE weekly_review_outlook (
    id uuid DEFAULT uuidv7() NOT NULL,
    weekly_review_id uuid NOT NULL,
    -- Which window this landing is over.
    period_kind text NOT NULL,
    -- The window's own local days, copied rather than re-derived: the reporting
    -- zone and the fiscal year start are operator-mutable settings, and
    -- re-cutting an old week's quarter later would re-label a landing that was
    -- already reported.
    period_start date NOT NULL,
    period_end date NOT NULL,

    -- Which snapshots this was read from, WITHOUT a foreign key. Retention may
    -- remove them; the figures below survive, and a dangling id is honest
    -- about where the numbers came from.
    opening_snapshot_id uuid,
    closing_snapshot_id uuid,

    -- The copied figures. Nullable as a GROUP: an opening side is absent when
    -- no Monday snapshot exists, and absent is not zero — a zero opening would
    -- draw a week that started from nothing and made everything.
    opening_landing_minor bigint,
    closing_landing_minor bigint,
    won_minor bigint,
    commit_minor bigint,
    best_case_minor bigint,
    weighted_minor bigint,
    -- Which measure the landing was built from, frozen: the installation
    -- setting can change, and a past week must keep saying what it was read
    -- under rather than silently re-meaning.
    forward_measure text,
    base_currency text NOT NULL,

    CONSTRAINT weekly_review_outlook_pkey PRIMARY KEY (id),
    CONSTRAINT weekly_review_outlook_period_kind_check
        CHECK (period_kind IN ('week', 'month', 'quarter')),
    -- A window that ends before it starts is a broken writer, not a short week.
    CONSTRAINT weekly_review_outlook_window_ordered CHECK (period_end >= period_start),
    -- The opening side travels together or not at all. Written with
    -- IS NOT DISTINCT FROM because a CHECK evaluating to NULL PASSES in
    -- Postgres, so a plain = would admit exactly the half-written row this
    -- refuses.
    CONSTRAINT weekly_review_outlook_opening_whole CHECK (
        (opening_snapshot_id IS NULL) IS NOT DISTINCT FROM (opening_landing_minor IS NULL)),
    -- A measure is present exactly when there is a landing to have been built
    -- by it.
    CONSTRAINT weekly_review_outlook_measure_with_landing CHECK (
        (forward_measure IS NULL) IS NOT DISTINCT FROM (closing_landing_minor IS NULL)),
    CONSTRAINT weekly_review_outlook_measure_known CHECK (
        forward_measure IS NULL
        OR forward_measure IN ('commit_evidence', 'weighted', 'manager_call')),
    CONSTRAINT weekly_review_outlook_currency_present CHECK (btrim(base_currency) <> '')
);

ALTER TABLE weekly_review_outlook
    ADD CONSTRAINT weekly_review_outlook_review_fkey FOREIGN KEY (weekly_review_id)
    REFERENCES weekly_review(id) ON DELETE CASCADE;

-- One landing per horizon per review. A second row for the same window would
-- give the panel two answers and no way to choose.
CREATE UNIQUE INDEX uq_weekly_review_outlook_period
    ON weekly_review_outlook (weekly_review_id, period_kind);

-- How the week got from its opening landing to its closing one.
--
-- The bars are the MOVES only. The two landings are the outlook row's own
-- anchors above; a bar for either would be summed as a movement as well as
-- read as a landing.
CREATE TABLE weekly_review_movement (
    id uuid DEFAULT uuidv7() NOT NULL,
    weekly_review_id uuid NOT NULL,
    period_kind text NOT NULL,
    -- Which bar. Folded from the forecast engine's twelve buckets by
    -- compose/weekly's own mapping, which a gate holds total.
    bar text NOT NULL,
    -- Signed: a bar can take money out of the period as well as put it in, and
    -- an unsigned magnitude would make a slip look like an advance.
    delta_minor bigint NOT NULL,

    CONSTRAINT weekly_review_movement_pkey PRIMARY KEY (id),
    CONSTRAINT weekly_review_movement_period_kind_check
        CHECK (period_kind IN ('week', 'month', 'quarter')),
    CONSTRAINT weekly_review_movement_bar_check
        CHECK (bar IN ('created', 'advanced', 'slipped', 'won', 'lost', 'other'))
);

ALTER TABLE weekly_review_movement
    ADD CONSTRAINT weekly_review_movement_review_fkey FOREIGN KEY (weekly_review_id)
    REFERENCES weekly_review(id) ON DELETE CASCADE;

CREATE UNIQUE INDEX uq_weekly_review_movement_bar
    ON weekly_review_movement (weekly_review_id, period_kind, bar);

-- The deals behind a bar, so a reader can ask "which ones?" of a number.
--
-- NO FOREIGN KEY ON deal_id, and the label is stored beside it, for the reason
-- weekly_review_deal states: a deal deleted next month must leave last month's
-- retrospective still saying what it said.
CREATE TABLE weekly_review_driver (
    id uuid DEFAULT uuidv7() NOT NULL,
    weekly_review_id uuid NOT NULL,
    period_kind text NOT NULL,
    bar text NOT NULL,
    deal_id uuid NOT NULL,
    -- What the deal was called that week.
    deal_label text NOT NULL,
    delta_minor bigint NOT NULL,

    CONSTRAINT weekly_review_driver_pkey PRIMARY KEY (id),
    CONSTRAINT weekly_review_driver_period_kind_check
        CHECK (period_kind IN ('week', 'month', 'quarter')),
    CONSTRAINT weekly_review_driver_bar_check
        CHECK (bar IN ('created', 'advanced', 'slipped', 'won', 'lost', 'other')),
    CONSTRAINT weekly_review_driver_label_present CHECK (btrim(deal_label) <> '')
);

ALTER TABLE weekly_review_driver
    ADD CONSTRAINT weekly_review_driver_review_fkey FOREIGN KEY (weekly_review_id)
    REFERENCES weekly_review(id) ON DELETE CASCADE;

CREATE INDEX idx_weekly_review_driver_bar
    ON weekly_review_driver (weekly_review_id, period_kind, bar);

-- One row per deal per bar. Without it a re-freeze duplicates every driver, and
-- a bar opened to "which deals?" answers with the same deal twice — the only
-- one of these three tables whose writer had no arbiter to conflict against.
CREATE UNIQUE INDEX uq_weekly_review_driver_deal
    ON weekly_review_driver (weekly_review_id, period_kind, bar, deal_id);

-- One period-close snapshot per scope per local day, however often the job
-- ticks.
--
-- The weekly job is at-least-once like every other consumer, so a retry that
-- took a second closing snapshot would give the next week two candidate
-- openings and no rule for picking one. PARTIAL, because only the period_close
-- trigger is once-a-day: a 'call' snapshot is taken whenever somebody calls,
-- and a 'daily' one has its own cadence.
--
-- Keyed on local_day, the column the snapshot table already stores for exactly
-- this. Casting taken_at::date here would compute the day in whatever zone the
-- writing session happens to carry, which is the bug that column exists to
-- prevent.
CREATE UNIQUE INDEX uq_forecast_snapshot_period_close
    ON forecast_snapshot (period_start, period_end, scope_kind, scope_id, local_day)
    WHERE trigger = 'period_close';

-- A partial index cannot cover a null scope_id, so the workspace scope needs
-- its own arbiter or this rule silently does not apply to it — the one scope
-- every installation has. The same pair as uq_forecast_snapshot_daily and its
-- workspace sibling, for the same reason.
CREATE UNIQUE INDEX uq_forecast_snapshot_period_close_workspace
    ON forecast_snapshot (period_start, period_end, local_day)
    WHERE trigger = 'period_close' AND scope_id IS NULL;
