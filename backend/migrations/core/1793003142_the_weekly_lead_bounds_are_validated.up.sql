-- Validates the two checks 1793003141 added NOT VALID. Its own
-- file, so VALIDATE runs in its own transaction under SHARE UPDATE EXCLUSIVE and
-- writers are not held behind the scan.
SET LOCAL lock_timeout = '5s';

ALTER TABLE weekly_review_scorecard
    VALIDATE CONSTRAINT weekly_review_scorecard_lead_cohort_fits;

ALTER TABLE team_weekly_review
    VALIDATE CONSTRAINT team_weekly_review_subsets;
