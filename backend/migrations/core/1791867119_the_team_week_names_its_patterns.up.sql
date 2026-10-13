-- The team's frozen week names every pattern per rep, its whole roster, its
-- operating quality, its coverage and the stages where a rep's funnel differs.
--
-- Additive. A snapshot written before this file has no child rows and NULL
-- quality columns, and the reader says "not measured" for it rather than zero.
--
-- The three constraints on tables this file did not create are added NOT VALID
-- and validated by the file after this one, so no write waits on the scan.
SET LOCAL lock_timeout = '3s';

-- How many of the rep's open deals had a provisional close date when the week
-- ended. NULL on a scorecard frozen before this column existed, so the rule
-- reading it cannot fire on a week that never measured it. Left out of the
-- deal block shape CHECK for that reason, as deal_unreconstructible is.
ALTER TABLE weekly_review_scorecard ADD COLUMN deal_close_date_provisional integer;
ALTER TABLE weekly_review_scorecard ADD CONSTRAINT weekly_review_scorecard_provisional_fits
    CHECK (deal_close_date_provisional IS NULL
        OR (deal_close_date_provisional >= 0 AND deal_open IS NOT NULL
            AND deal_close_date_provisional <= deal_open)) NOT VALID;

-- deals_at_risk stays in the vocabulary: snapshots written since it was added
-- carry it, and a migration may not rewrite them.
ALTER TABLE team_weekly_review_rep DROP CONSTRAINT team_weekly_review_rep_focus_kind_check;
ALTER TABLE team_weekly_review_rep ADD CONSTRAINT team_weekly_review_rep_focus_kind_check
    CHECK (focus_kind IN ('help_requested', 'leads_breached', 'commitments_missed',
        'meetings_without_next_step', 'deals_at_risk', 'strong_week', 'quiet_week',
        'no_next_step_coverage', 'close_dates_provisional', 'forecast_downgrades',
        'stage_regressions')) NOT VALID;

-- Every pattern that fired for one rep, in the order a lead raises them.
-- Position 0 is always the rep row's own focus_kind and focus_label.
CREATE TABLE team_weekly_review_rep_pattern (
    id uuid DEFAULT uuidv7() NOT NULL,
    team_weekly_review_rep_id uuid NOT NULL,
    position integer NOT NULL,
    kind text NOT NULL,
    label text NOT NULL,
    CONSTRAINT team_weekly_review_rep_pattern_pkey PRIMARY KEY (id),
    CONSTRAINT team_weekly_review_rep_pattern_rep_fkey FOREIGN KEY (team_weekly_review_rep_id)
        REFERENCES team_weekly_review_rep(id) ON DELETE CASCADE,
    CONSTRAINT team_weekly_review_rep_pattern_kind_check
        CHECK (kind IN ('help_requested', 'leads_breached', 'commitments_missed',
            'meetings_without_next_step', 'deals_at_risk', 'strong_week', 'quiet_week',
            'no_next_step_coverage', 'close_dates_provisional', 'forecast_downgrades',
            'stage_regressions')),
    CONSTRAINT team_weekly_review_rep_pattern_position_check CHECK (position >= 0),
    CONSTRAINT team_weekly_review_rep_pattern_label_present CHECK (btrim(label) <> ''),
    CONSTRAINT uq_team_weekly_review_rep_pattern_position UNIQUE (team_weekly_review_rep_id, position),
    CONSTRAINT uq_team_weekly_review_rep_pattern_kind UNIQUE (team_weekly_review_rep_id, kind)
);

-- Every live member of the team that week, including the ones with no review
-- of their own. The rep rows are the measured subset; this is the roster the
-- live exceptions and the funnel are read over.
CREATE TABLE team_weekly_review_member (
    id uuid DEFAULT uuidv7() NOT NULL,
    team_weekly_review_id uuid NOT NULL,
    user_id uuid NOT NULL,
    display_name text NOT NULL,
    measured boolean NOT NULL,
    CONSTRAINT team_weekly_review_member_pkey PRIMARY KEY (id),
    CONSTRAINT team_weekly_review_member_review_fkey FOREIGN KEY (team_weekly_review_id)
        REFERENCES team_weekly_review(id) ON DELETE CASCADE,
    CONSTRAINT team_weekly_review_member_name_present CHECK (btrim(display_name) <> ''),
    CONSTRAINT uq_team_weekly_review_member UNIQUE (team_weekly_review_id, user_id)
);

-- Operating quality, frozen with the week. NULL on an older snapshot, which
-- reads as "not measured" rather than as a week with nothing in it.
ALTER TABLE team_weekly_review
    ADD COLUMN proposals_accepted integer,
    ADD COLUMN proposals_rejected integer,
    ADD COLUMN auto_change_reversals integer,
    ADD COLUMN summary text;
ALTER TABLE team_weekly_review ADD CONSTRAINT team_weekly_review_quality_tallies
    CHECK ((proposals_accepted IS NULL OR proposals_accepted >= 0)
       AND (proposals_rejected IS NULL OR proposals_rejected >= 0)
       AND (auto_change_reversals IS NULL OR auto_change_reversals >= 0)) NOT VALID;

-- Whether the team's open pipeline covered what the month and the quarter
-- need, against a named basis. Exactly one of basis and absent is set.
CREATE TABLE team_weekly_review_coverage (
    id uuid DEFAULT uuidv7() NOT NULL,
    team_weekly_review_id uuid NOT NULL,
    period_kind text NOT NULL,
    basis text,
    absent text,
    reference_landing_minor bigint,
    needed_open_minor bigint,
    current_open_minor bigint,
    remaining_to_support_minor bigint,
    coverage_bp bigint,
    base_currency text NOT NULL,
    replan_recommended boolean NOT NULL,
    CONSTRAINT team_weekly_review_coverage_pkey PRIMARY KEY (id),
    CONSTRAINT team_weekly_review_coverage_review_fkey FOREIGN KEY (team_weekly_review_id)
        REFERENCES team_weekly_review(id) ON DELETE CASCADE,
    CONSTRAINT team_weekly_review_coverage_period_kind_check
        CHECK (period_kind IN ('month', 'quarter')),
    CONSTRAINT team_weekly_review_coverage_basis_check
        CHECK (basis IS NULL OR basis IN ('manager_call', 'historical_median')),
    CONSTRAINT team_weekly_review_coverage_absent_check
        CHECK (absent IS NULL OR absent IN ('insufficient_basis', 'insufficient_history', 'not_authorized')),
    CONSTRAINT team_weekly_review_coverage_basis_or_absent
        CHECK ((basis IS NULL) <> (absent IS NULL)),
    -- A named basis carries every figure, so a reader never draws a ratio
    -- with half of its inputs missing.
    CONSTRAINT team_weekly_review_coverage_basis_whole CHECK (
        basis IS NULL OR (reference_landing_minor IS NOT NULL AND needed_open_minor IS NOT NULL
            AND current_open_minor IS NOT NULL AND remaining_to_support_minor IS NOT NULL
            AND coverage_bp IS NOT NULL)),
    -- An absence carries no figure: a call the member could not read must not
    -- reach a reader as a number beside the word that withheld it.
    CONSTRAINT team_weekly_review_coverage_absent_bare CHECK (
        absent IS NULL OR (reference_landing_minor IS NULL AND needed_open_minor IS NULL
            AND current_open_minor IS NULL AND remaining_to_support_minor IS NULL
            AND coverage_bp IS NULL)),
    -- Total comparisons: a CHECK that evaluates to NULL passes, so a plain =
    -- would admit a raised flag with no basis behind it.
    CONSTRAINT team_weekly_review_coverage_replan_follows CHECK (
        replan_recommended IS NOT DISTINCT FROM
            (basis IS NOT NULL AND coverage_bp IS NOT NULL AND coverage_bp < 8000)),
    CONSTRAINT team_weekly_review_coverage_currency_check CHECK (base_currency ~ '^[A-Z]{3}$'),
    CONSTRAINT team_weekly_review_coverage_reference_landing_minor_js_safe
        CHECK (reference_landing_minor BETWEEN -9007199254740991 AND 9007199254740991),
    CONSTRAINT team_weekly_review_coverage_needed_open_minor_js_safe
        CHECK (needed_open_minor BETWEEN -9007199254740991 AND 9007199254740991),
    CONSTRAINT team_weekly_review_coverage_current_open_minor_js_safe
        CHECK (current_open_minor BETWEEN -9007199254740991 AND 9007199254740991),
    CONSTRAINT team_weekly_review_coverage_remaining_to_support_minor_js_safe
        CHECK (remaining_to_support_minor BETWEEN -9007199254740991 AND 9007199254740991),
    CONSTRAINT team_weekly_review_coverage_coverage_bp_js_safe
        CHECK (coverage_bp BETWEEN -9007199254740991 AND 9007199254740991),
    CONSTRAINT uq_team_weekly_review_coverage_period UNIQUE (team_weekly_review_id, period_kind)
);

-- Up to three stages where the rep's conversion differs most from the team's.
-- Names are frozen and carry no foreign key, so a renamed or deleted stage
-- leaves the week as it read. "Converted" compares stage positions as they
-- were configured when the week was assembled, because positions are not
-- kept in the stage history.
CREATE TABLE team_weekly_review_rep_funnel (
    id uuid DEFAULT uuidv7() NOT NULL,
    team_weekly_review_rep_id uuid NOT NULL,
    position integer NOT NULL,
    pipeline_id uuid NOT NULL,
    pipeline_name text NOT NULL,
    stage_id uuid NOT NULL,
    stage_name text NOT NULL,
    next_stage_name text,
    rep_entered integer NOT NULL,
    rep_converted integer NOT NULL,
    team_entered integer NOT NULL,
    team_converted integer NOT NULL,
    deviation_bp integer NOT NULL,
    CONSTRAINT team_weekly_review_rep_funnel_pkey PRIMARY KEY (id),
    CONSTRAINT team_weekly_review_rep_funnel_rep_fkey FOREIGN KEY (team_weekly_review_rep_id)
        REFERENCES team_weekly_review_rep(id) ON DELETE CASCADE,
    CONSTRAINT team_weekly_review_rep_funnel_position_check CHECK (position BETWEEN 1 AND 3),
    CONSTRAINT team_weekly_review_rep_funnel_names_present
        CHECK (btrim(pipeline_name) <> '' AND btrim(stage_name) <> ''),
    CONSTRAINT team_weekly_review_rep_funnel_subsets CHECK (
        rep_converted >= 0 AND rep_converted <= rep_entered
        AND team_converted >= 0 AND team_converted <= team_entered
        AND rep_entered <= team_entered AND rep_converted <= team_converted),
    CONSTRAINT team_weekly_review_rep_funnel_deviation_check CHECK (deviation_bp BETWEEN -10000 AND 10000),
    CONSTRAINT uq_team_weekly_review_rep_funnel_position UNIQUE (team_weekly_review_rep_id, position)
);
