-- Bounded, because ALTER TABLE takes an ACCESS EXCLUSIVE lock and a weekly
-- scorecard is written by a job that runs while people are reading.
SET LOCAL lock_timeout = '5s';

ALTER TABLE weekly_review_scorecard
    DROP COLUMN lead_arrived;
