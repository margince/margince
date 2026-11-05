-- Put both checks back the way this migration found them: present, unvalidated.
-- Postgres has no ALTER that un-validates, so each is dropped and re-added NOT
-- VALID, which takes ACCESS EXCLUSIVE without a scan.
SET LOCAL lock_timeout = '5s';

ALTER TABLE weekly_review_scorecard
    DROP CONSTRAINT weekly_review_scorecard_lead_cohort_fits,
    ADD CONSTRAINT weekly_review_scorecard_lead_cohort_fits CHECK (
        lead_arrived IS NULL
        OR (lead_arrived >= 0
            AND has_lead_block
            AND lead_answered_in_target <= lead_arrived
            AND lead_breached <= lead_arrived)) NOT VALID;

ALTER TABLE team_weekly_review
    DROP CONSTRAINT team_weekly_review_subsets,
    ADD CONSTRAINT team_weekly_review_subsets CHECK (
        meetings_with_next_step <= meetings_held
        AND commitments_kept <= commitments_due
        AND leads_answered_in_target <= leads_routed
        AND leads_breached <= leads_routed) NOT VALID;
