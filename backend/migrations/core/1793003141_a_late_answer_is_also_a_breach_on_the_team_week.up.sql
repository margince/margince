-- A lead answered after its target is both answered and breached, and the
-- frozen lead cohort bounds both figures.
--
-- team_weekly_review_subsets required answered + breached <= routed, as if the
-- two were disjoint. They are not: leads_answered_in_target counts every first
-- response recorded by the close, late ones included, and leads_breached
-- counts every breach recorded by then. One lead answered late is in both, so a
-- team week holding one would be refused at freeze. Each figure is bounded by
-- the cohort on its own instead.
--
-- Replaced NOT VALID so no scan runs under the lock; 1793003142 validates it.
SET LOCAL lock_timeout = '5s';

ALTER TABLE team_weekly_review
    DROP CONSTRAINT IF EXISTS team_weekly_review_subsets;

ALTER TABLE team_weekly_review
    ADD CONSTRAINT team_weekly_review_subsets CHECK (
        meetings_with_next_step <= meetings_held
        AND commitments_kept <= commitments_due
        AND leads_answered_in_target <= leads_routed
        AND leads_breached <= leads_routed) NOT VALID;

-- The week's lead cohort (1793003140) is a tally, belongs to a lead block, and
-- holds every lead the two figures beside it were counted from, each on its own.
-- NULL is a week frozen before the cohort was recorded.
ALTER TABLE weekly_review_scorecard
    ADD CONSTRAINT weekly_review_scorecard_lead_cohort_fits CHECK (
        lead_arrived IS NULL
        OR (lead_arrived >= 0
            AND has_lead_block
            AND lead_answered_in_target <= lead_arrived
            AND lead_breached <= lead_arrived)) NOT VALID;
