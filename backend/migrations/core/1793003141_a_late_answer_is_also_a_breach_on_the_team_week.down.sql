-- Restores the disjoint bound. NOT VALID: rows written under the looser check
-- may hold a late answer counted twice, and a rollback must not fail on them.
SET LOCAL lock_timeout = '5s';

ALTER TABLE team_weekly_review
    DROP CONSTRAINT IF EXISTS team_weekly_review_subsets;

ALTER TABLE team_weekly_review
    ADD CONSTRAINT team_weekly_review_subsets CHECK (
        meetings_with_next_step <= meetings_held
        AND commitments_kept <= commitments_due
        AND leads_answered_in_target + leads_breached <= leads_routed) NOT VALID;

ALTER TABLE weekly_review_scorecard
    DROP CONSTRAINT IF EXISTS weekly_review_scorecard_lead_cohort_fits;
