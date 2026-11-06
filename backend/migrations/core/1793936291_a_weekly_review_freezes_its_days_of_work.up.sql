-- Each day of the rep's own work, frozen with the review like its counts.
-- Nullable: a review written before this has no days and states none.
SET LOCAL lock_timeout = '3s';

ALTER TABLE weekly_review ADD COLUMN daily_activity jsonb;
