-- What the week's work came to, frozen beside the review that reports it.
--
-- The counts already on weekly_review say what HAPPENED — leads routed,
-- meetings held, deals moved. This says how WELL, and the two are different
-- questions: "you were sent 40 leads" is a fact about the week, "you answered
-- 12 of them inside the target" is a judgement of it.
--
-- FROZEN for the reason the whole review is frozen. A scorecard recomputed on
-- read answers differently every time somebody edits an old deal, and a rep
-- comparing this week against March would be comparing today's arithmetic
-- against today's arithmetic rather than two weeks.
--
-- PRESENCE IS NOT ZERO, and that is what the two booleans buy. A rep who
-- carried no leads at all did not score 0% on the funnel — the funnel is not
-- their job that week, and drawing them a row of zeros reads as failure at
-- something they were never asked to do. So each block is present or absent,
-- and a reader draws only what is present.
SET LOCAL lock_timeout = '3s';

CREATE TABLE weekly_review_scorecard (
    id uuid DEFAULT uuidv7() NOT NULL,
    weekly_review_id uuid NOT NULL,

    -- Whether this week had a funnel at all. False means the rep carried no
    -- lead in the window, so every lead_* column below is NULL and the panel
    -- draws no lead block.
    has_lead_block boolean NOT NULL,
    -- Status transitions counted from audit_log's before/after images, which is
    -- where a lead's movement actually lives: the lead row carries its CURRENT
    -- status and a handful of milestone stamps, so it cannot answer "who moved
    -- from contacted to engaged last week". Every one of the five writers of
    -- lead.status goes through storekit.Audit, so the images are complete.
    lead_advanced integer,
    lead_disqualified integer,
    lead_promoted integer,
    -- The SLA slice of the same week, copied from the review's own counts so
    -- the scorecard reads as one record rather than sending a reader to two.
    lead_answered_in_target integer,
    lead_breached integer,
    -- Meetings by the status they BECAME this week, read from
    -- activity_meeting_history and never from activity.meeting_status: a
    -- meeting booked Monday and held Friday reads `held` today, so counting the
    -- column reports no bookings for the week it was booked in.
    meetings_booked integer,
    meetings_held integer,
    meetings_no_show integer,
    -- Meetings whose history is a row the backfill invented from current state.
    -- They cannot say when they were booked, so coverage is reported as partial
    -- rather than counting them as history.
    meetings_partial_history integer,

    -- Whether this week had deals to score. False means no open deal and no
    -- stage change in the window.
    has_deal_block boolean NOT NULL,
    deal_advances integer,
    deal_regressions integer,
    -- Median whole days a deal sat in the stage it left this week. NULL with a
    -- present deal block means no deal changed stage — a median of nothing is
    -- absent, never zero.
    deal_median_days_in_stage integer,
    -- Open deals carrying an open task, over open deals. Counts, not a rate, so
    -- a reader can see the denominator rather than trust a percentage.
    deal_with_next_step integer,
    deal_open integer,
    -- Open deals with at least two distinct people linked in the last 30 days.
    deal_multi_threaded integer,
    -- Open deals whose close date is set and not in the past.
    deal_close_date_sound integer,
    -- Forecast category moves, each deal counted ONCE however many times it was
    -- edited: a rep who corrected a typo three times did not downgrade thrice.
    deal_forecast_up integer,
    deal_forecast_down integer,

    created_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT weekly_review_scorecard_pkey PRIMARY KEY (id),
    CONSTRAINT weekly_review_scorecard_review_fkey
        FOREIGN KEY (weekly_review_id) REFERENCES weekly_review(id) ON DELETE CASCADE,
    -- One scorecard per review. The review's own insert is the arbiter of a
    -- second assembly, but freezing is reachable on its own, so the constraint
    -- is here rather than assumed upstream.
    CONSTRAINT weekly_review_scorecard_one_per_review UNIQUE (weekly_review_id),

    -- A block's columns are present exactly when the block is. Spelled with
    -- IS NOT DISTINCT FROM because a CHECK evaluating to NULL PASSES in
    -- Postgres, so `= false` would admit precisely the rows this refuses.
    CONSTRAINT weekly_review_scorecard_lead_block_shape CHECK (
        (has_lead_block IS NOT DISTINCT FROM true) = (lead_advanced IS NOT NULL)
        AND (lead_advanced IS NOT NULL) = (lead_disqualified IS NOT NULL)
        AND (lead_advanced IS NOT NULL) = (lead_promoted IS NOT NULL)
        AND (lead_advanced IS NOT NULL) = (lead_answered_in_target IS NOT NULL)
        AND (lead_advanced IS NOT NULL) = (lead_breached IS NOT NULL)
        AND (lead_advanced IS NOT NULL) = (meetings_booked IS NOT NULL)
        AND (lead_advanced IS NOT NULL) = (meetings_held IS NOT NULL)
        AND (lead_advanced IS NOT NULL) = (meetings_no_show IS NOT NULL)
        AND (lead_advanced IS NOT NULL) = (meetings_partial_history IS NOT NULL)
    ),
    CONSTRAINT weekly_review_scorecard_deal_block_shape CHECK (
        (has_deal_block IS NOT DISTINCT FROM true) = (deal_advances IS NOT NULL)
        AND (deal_advances IS NOT NULL) = (deal_regressions IS NOT NULL)
        AND (deal_advances IS NOT NULL) = (deal_with_next_step IS NOT NULL)
        AND (deal_advances IS NOT NULL) = (deal_open IS NOT NULL)
        AND (deal_advances IS NOT NULL) = (deal_multi_threaded IS NOT NULL)
        AND (deal_advances IS NOT NULL) = (deal_close_date_sound IS NOT NULL)
        AND (deal_advances IS NOT NULL) = (deal_forecast_up IS NOT NULL)
        AND (deal_advances IS NOT NULL) = (deal_forecast_down IS NOT NULL)
    ),
    -- The median is the one deal column that may be absent inside a present
    -- block, so it is excluded from the shape above and bounded here instead.
    CONSTRAINT weekly_review_scorecard_median_needs_block CHECK (
        deal_median_days_in_stage IS NULL OR has_deal_block IS NOT DISTINCT FROM true
    ),
    -- No count is negative. A negative would mean the arithmetic above it went
    -- wrong, and storing it would put the mistake in the frozen record.
    CONSTRAINT weekly_review_scorecard_counts_nonnegative CHECK (
        COALESCE(lead_advanced, 0) >= 0 AND COALESCE(lead_disqualified, 0) >= 0
        AND COALESCE(lead_promoted, 0) >= 0 AND COALESCE(lead_answered_in_target, 0) >= 0
        AND COALESCE(lead_breached, 0) >= 0 AND COALESCE(meetings_booked, 0) >= 0
        AND COALESCE(meetings_held, 0) >= 0 AND COALESCE(meetings_no_show, 0) >= 0
        AND COALESCE(meetings_partial_history, 0) >= 0
        AND COALESCE(deal_advances, 0) >= 0 AND COALESCE(deal_regressions, 0) >= 0
        AND COALESCE(deal_median_days_in_stage, 0) >= 0
        AND COALESCE(deal_with_next_step, 0) >= 0 AND COALESCE(deal_open, 0) >= 0
        AND COALESCE(deal_multi_threaded, 0) >= 0
        AND COALESCE(deal_close_date_sound, 0) >= 0
        AND COALESCE(deal_forecast_up, 0) >= 0 AND COALESCE(deal_forecast_down, 0) >= 0
    ),
    -- A subset can never exceed its own denominator.
    CONSTRAINT weekly_review_scorecard_subsets_fit CHECK (
        (deal_open IS NULL OR (
            deal_with_next_step <= deal_open
            AND deal_multi_threaded <= deal_open
            AND deal_close_date_sound <= deal_open))
    )
);
