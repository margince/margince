-- The team's week, frozen the way each rep's already is.
--
-- WHY IT IS STORED RATHER THAN SUMMED ON READ. Every figure here is a total
-- over the member reviews, so a read-time SUM would be simpler — and wrong.
-- Team membership moves: a rep who joins in October would silently appear in
-- last March's team week, one who leaves would silently vanish from it, and a
-- lead comparing two quarters would be comparing two different teams without
-- being told. The same reason weekly_review freezes its own counts rather than
-- recomputing them, and the same reason weekly_review_deal stores the label
-- beside the id.
--
-- So the membership is frozen INTO the snapshot: the rep rows below name who
-- was on the team that week, with the display name they had then.
SET LOCAL lock_timeout = '5s';

CREATE TABLE team_weekly_review (
    id uuid DEFAULT uuidv7() NOT NULL,
    team_id uuid NOT NULL,
    -- The Monday under review, in the installation's reporting zone — the same
    -- shape and zone weekly_review.local_week_start uses, so a team week and
    -- the rep weeks inside it name the same seven days.
    local_week_start date NOT NULL,
    -- The team's name that week. Frozen for the reason every label here is: a
    -- team renamed in June must not relabel March's snapshot.
    team_name text NOT NULL,
    generated_at timestamptz DEFAULT now() NOT NULL,
    as_of timestamptz NOT NULL,

    -- The team's own totals, summed over the member reviews at write time.
    reps_counted int DEFAULT 0 NOT NULL,
    deals_won int DEFAULT 0 NOT NULL,
    deals_lost int DEFAULT 0 NOT NULL,
    leads_routed int DEFAULT 0 NOT NULL,
    leads_answered_in_target int DEFAULT 0 NOT NULL,
    leads_breached int DEFAULT 0 NOT NULL,
    meetings_held int DEFAULT 0 NOT NULL,
    meetings_with_next_step int DEFAULT 0 NOT NULL,
    commitments_due int DEFAULT 0 NOT NULL,
    commitments_kept int DEFAULT 0 NOT NULL,

    -- Money the team's week moved, in the installation's base currency.
    --
    -- NULLABLE together with its currency, and for the reason the per-rep
    -- figures are: a member week that could not be converted makes the team
    -- total unanswerable too. Summing the ones that DID convert would be a
    -- confident number quietly missing a rep, which is worse than an absent
    -- one — and a lead comparing two weeks would not be told.
    pipeline_created_minor bigint,
    pipeline_won_minor bigint,
    pipeline_lost_minor bigint,
    base_currency text,

    -- Which member weeks this snapshot could not read at all.
    --
    -- A count rather than a list: the names are in the rep rows, and a team
    -- week that silently covered four of six reps reads exactly like a team of
    -- four. Zero is the claim that every member's week was counted.
    reps_unread int DEFAULT 0 NOT NULL,

    CONSTRAINT team_weekly_review_pkey PRIMARY KEY (id),
    CONSTRAINT team_weekly_review_counts_are_tallies CHECK (
        reps_counted >= 0 AND reps_unread >= 0
        AND deals_won >= 0 AND deals_lost >= 0
        AND leads_routed >= 0 AND leads_answered_in_target >= 0 AND leads_breached >= 0
        AND meetings_held >= 0 AND meetings_with_next_step >= 0
        AND commitments_due >= 0 AND commitments_kept >= 0
        AND (pipeline_created_minor IS NULL OR pipeline_created_minor >= 0)
        AND (pipeline_won_minor IS NULL OR pipeline_won_minor >= 0)
        AND (pipeline_lost_minor IS NULL OR pipeline_lost_minor >= 0)),
    -- A subset is never larger than the set it is drawn from.
    CONSTRAINT team_weekly_review_subsets CHECK (
        meetings_with_next_step <= meetings_held
        AND commitments_kept <= commitments_due
        AND leads_answered_in_target + leads_breached <= leads_routed),
    -- A converted figure names its currency, and a currency names figures.
    CONSTRAINT team_weekly_review_money_names_its_currency CHECK (
        (base_currency IS NULL) = (pipeline_created_minor IS NULL
                                   AND pipeline_won_minor IS NULL
                                   AND pipeline_lost_minor IS NULL)),
    CONSTRAINT team_weekly_review_currency_check CHECK (
        base_currency IS NULL OR base_currency ~ '^[A-Z]{3}$'),
    CONSTRAINT team_weekly_review_name_present CHECK (btrim(team_name) <> '')
);

-- One snapshot per team per week. The same arbiter weekly_review uses: the job
-- runs more than once inside a week so a worker that was down still backfills,
-- and this collapses those runs into one.
ALTER TABLE team_weekly_review
    ADD CONSTRAINT uq_team_weekly_review_team_week UNIQUE (team_id, local_week_start);

-- NO FOREIGN KEY on team_id, deliberately, and the same ruling
-- weekly_review_deal makes about deal_id: a team disbanded in June must leave
-- March's snapshot saying what it said. team_name is stored beside the id for
-- exactly that case.

CREATE INDEX idx_team_weekly_review_team
    ON team_weekly_review (team_id, local_week_start DESC);

-- One row per rep who was on the team that week.
--
-- This IS the frozen membership. Reading it back tells a lead who was on the
-- team in March, which a join against team_membership could never answer.
CREATE TABLE team_weekly_review_rep (
    id uuid DEFAULT uuidv7() NOT NULL,
    team_weekly_review_id uuid NOT NULL,
    user_id uuid NOT NULL,
    -- What they were called that week. Frozen for the reason every label here
    -- is, and it also survives the seat being deleted.
    display_name text NOT NULL,

    deals_won int DEFAULT 0 NOT NULL,
    leads_breached int DEFAULT 0 NOT NULL,
    meetings_held int DEFAULT 0 NOT NULL,
    commitments_due int DEFAULT 0 NOT NULL,
    commitments_kept int DEFAULT 0 NOT NULL,
    help_requested int DEFAULT 0 NOT NULL,

    -- The one thing this rep's lead should raise with them, and which rule
    -- picked it. Every team has one per rep — including a rep whose week went
    -- well, whose focus is the thing the team should copy.
    focus_kind text NOT NULL,
    focus_label text NOT NULL,

    CONSTRAINT team_weekly_review_rep_pkey PRIMARY KEY (id),
    CONSTRAINT team_weekly_review_rep_counts_are_tallies CHECK (
        deals_won >= 0 AND leads_breached >= 0 AND meetings_held >= 0
        AND commitments_due >= 0 AND commitments_kept >= 0 AND help_requested >= 0
        AND commitments_kept <= commitments_due),
    CONSTRAINT team_weekly_review_rep_name_present CHECK (btrim(display_name) <> ''),
    CONSTRAINT team_weekly_review_rep_focus_present CHECK (btrim(focus_label) <> ''),
    -- The rule that fired, so a reader can tell a coaching prompt from a thing
    -- to celebrate — and so a test can assert the positive fallback is
    -- reachable rather than trusting that it is.
    CONSTRAINT team_weekly_review_rep_focus_kind_check CHECK (
        focus_kind IN ('help_requested', 'leads_breached', 'commitments_missed',
                       'meetings_without_next_step', 'strong_week', 'quiet_week'))
);

ALTER TABLE team_weekly_review_rep
    ADD CONSTRAINT team_weekly_review_rep_review_fkey FOREIGN KEY (team_weekly_review_id)
    REFERENCES team_weekly_review(id) ON DELETE CASCADE;

-- One row per rep per snapshot: a second would double every team total drawn
-- from these rows.
ALTER TABLE team_weekly_review_rep
    ADD CONSTRAINT uq_team_weekly_review_rep UNIQUE (team_weekly_review_id, user_id);

CREATE INDEX idx_team_weekly_review_rep_review
    ON team_weekly_review_rep (team_weekly_review_id, display_name);
