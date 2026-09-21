-- Bounded, because ALTER TABLE takes an ACCESS EXCLUSIVE lock and a weekly
-- scorecard is written by a job that runs while people are reading. Waiting
-- unbounded behind one long reader would queue every writer behind this.
SET LOCAL lock_timeout = '5s';

ALTER TABLE weekly_review_scorecard
    DROP COLUMN deal_unreconstructible;
