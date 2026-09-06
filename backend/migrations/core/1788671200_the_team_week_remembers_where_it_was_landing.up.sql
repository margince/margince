-- Where the TEAM's week was landing, frozen beside the snapshot that reports it.
--
-- The same fact the rep's weekly_review_outlook holds, over a different book: a
-- lead reads their team's landing, not the sum of six personal ones, because
-- the two differ wherever a deal is owned by nobody on the team or by two
-- people at once.
--
-- A SEPARATE TABLE and not a nullable column on the rep's, because the parent
-- differs: that one hangs off weekly_review by foreign key and this hangs off
-- team_weekly_review. The COLUMNS are a deliberate mirror — same names, same
-- types, same invariants — and TestTheTeamOutlookMirrorsTheRepOutlook holds
-- them together in both directions, so a column added to one and not the other
-- fails rather than drifting.
SET LOCAL lock_timeout = '3s';

CREATE TABLE team_weekly_review_outlook (
    id uuid DEFAULT uuidv7() NOT NULL,
    team_weekly_review_id uuid NOT NULL,
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

    CONSTRAINT team_weekly_review_outlook_pkey PRIMARY KEY (id),
    CONSTRAINT team_weekly_review_outlook_period_kind_check
        CHECK (period_kind IN ('week', 'month', 'quarter')),
    -- A window that ends before it starts is a broken writer, not a short week.
    CONSTRAINT team_weekly_review_outlook_window_ordered CHECK (period_end >= period_start),
    -- The opening side travels together or not at all. Written with
    -- IS NOT DISTINCT FROM because a CHECK evaluating to NULL PASSES in
    -- Postgres, so a plain = would admit exactly the half-written row this
    -- refuses.
    CONSTRAINT team_weekly_review_outlook_opening_whole CHECK (
        (opening_snapshot_id IS NULL) IS NOT DISTINCT FROM (opening_landing_minor IS NULL)),
    -- A measure is present exactly when there is a landing to have been built
    -- by it.
    CONSTRAINT team_weekly_review_outlook_measure_with_landing CHECK (
        (forward_measure IS NULL) IS NOT DISTINCT FROM (closing_landing_minor IS NULL)),
    CONSTRAINT team_weekly_review_outlook_measure_known CHECK (
        forward_measure IS NULL
        OR forward_measure IN ('commit_evidence', 'weighted', 'manager_call')),
    CONSTRAINT team_weekly_review_outlook_currency_present CHECK (btrim(base_currency) <> '')
);

ALTER TABLE team_weekly_review_outlook
    ADD CONSTRAINT team_weekly_review_outlook_review_fkey
    FOREIGN KEY (team_weekly_review_id)
    REFERENCES team_weekly_review(id) ON DELETE CASCADE;

-- One landing per horizon per snapshot. A second row for the same window would
-- give the panel two answers and no way to choose.
CREATE UNIQUE INDEX uq_team_weekly_review_outlook_period
    ON team_weekly_review_outlook (team_weekly_review_id, period_kind);
