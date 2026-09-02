-- A week that says only what happened, never what changed.
--
-- weekly_review counts tasks, deals and proposals, and a rep reading one
-- learns what their week held. What they cannot learn is whether it was a
-- better week than the last one — "12 deals moved" is a fact with no bar
-- against it, and a retrospective whose numbers cannot be compared is a
-- report rather than a review.
--
-- Two additions. The comparison columns below record what the existing
-- counts never had — how the week's leads were answered, whether its
-- meetings produced a next step, what it added to and took out of the
-- pipeline. And prior_review_id names the row this week is measured
-- against, so the delta is a subtraction between two FROZEN rows rather
-- than a live re-read of a week that has since moved on.
--
-- FROZEN AT WRITE, like every other column here. The rates, the labels and
-- the counts are what they were when the job ran; a deal renamed or
-- reopened next month does not rewrite last month's review. The deltas
-- themselves are NOT stored: two frozen rows and one subtraction cannot
-- disagree, and a stored delta could.
SET LOCAL lock_timeout = '5s';

ALTER TABLE weekly_review
    -- How the week's inbound leads were answered. Routed counts the leads
    -- that arrived; answered_in_target and breached split them by whether
    -- the first reply beat the policy. A lead still inside its target at
    -- write time is in neither — it has not yet been either.
    ADD COLUMN leads_routed int DEFAULT 0 NOT NULL,
    ADD COLUMN leads_answered_in_target int DEFAULT 0 NOT NULL,
    ADD COLUMN leads_breached int DEFAULT 0 NOT NULL,

    -- Meetings held, and how many left the record with a next step on it.
    -- The second is the one a rep can act on: a week of meetings that
    -- produced no follow-up is the pattern this column exists to make
    -- visible.
    ADD COLUMN meetings_held int DEFAULT 0 NOT NULL,
    ADD COLUMN meetings_with_next_step int DEFAULT 0 NOT NULL,

    -- What the week did to the pipeline, in the installation's base
    -- currency at the rate that applied when the row was written.
    --
    -- NULLABLE, and the nulls are load-bearing. An open deal holds no
    -- frozen rate — deal_closed_fx only binds a closed one — so a deal in a
    -- currency with no usable fx_rate cannot be converted at all. Such a
    -- week records NULL rather than a total that silently omits it: a
    -- confident figure that is quietly short is worse than an absent one,
    -- and nothing is ever converted at an invented rate of 1, which would
    -- report ¥5,000,000 as €5,000,000.
    ADD COLUMN pipeline_created_minor bigint,
    ADD COLUMN pipeline_won_minor bigint,
    ADD COLUMN pipeline_lost_minor bigint,

    -- The currency the three figures above are in. Stored beside them
    -- because the installation's base currency is an operator-mutable
    -- setting: re-reading it later would re-label old reviews with a
    -- currency their numbers were never in.
    ADD COLUMN base_currency text,

    -- The week this one is compared against — the same rep's previous
    -- review, or NULL for their first.
    --
    -- A stored pointer rather than a lookup by date arithmetic, because a
    -- rep with a gap in their history (a leave, a worker outage) has a
    -- previous review that is not last week, and "the week before this
    -- one" would find nothing and report every count as new.
    ADD COLUMN prior_review_id uuid,

    -- A tally is never negative; a negative one is a broken writer rather
    -- than a small week. Money can be zero and is never negative here —
    -- these are gross additions and closures, not a net movement.
    ADD CONSTRAINT weekly_review_new_counts_are_tallies CHECK (
        leads_routed >= 0 AND leads_answered_in_target >= 0 AND leads_breached >= 0
        AND meetings_held >= 0 AND meetings_with_next_step >= 0
        AND (pipeline_created_minor IS NULL OR pipeline_created_minor >= 0)
        AND (pipeline_won_minor IS NULL OR pipeline_won_minor >= 0)
        AND (pipeline_lost_minor IS NULL OR pipeline_lost_minor >= 0)),

    -- A meeting that produced a next step is a meeting that was held.
    ADD CONSTRAINT weekly_review_next_steps_were_meetings CHECK (
        meetings_with_next_step <= meetings_held),

    -- A converted figure names its currency, and a currency names figures.
    -- Either alone is a number nobody can read: an amount in no stated
    -- currency, or a currency label over nothing.
    ADD CONSTRAINT weekly_review_money_names_its_currency CHECK (
        (base_currency IS NULL) = (pipeline_created_minor IS NULL
                                   AND pipeline_won_minor IS NULL
                                   AND pipeline_lost_minor IS NULL)),

    ADD CONSTRAINT weekly_review_currency_check CHECK (
        base_currency IS NULL OR base_currency ~ '^[A-Z]{3}$');

-- ON DELETE SET NULL, not CASCADE: a rep's oldest review being erased must
-- not take the week that referenced it. The pointer is a comparison, not an
-- ownership.
ALTER TABLE weekly_review
    ADD CONSTRAINT weekly_review_prior_fkey FOREIGN KEY (prior_review_id)
    REFERENCES weekly_review(id) ON DELETE SET NULL;
