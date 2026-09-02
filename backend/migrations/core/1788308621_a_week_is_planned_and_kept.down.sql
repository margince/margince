SET LOCAL lock_timeout = '5s';

ALTER TABLE weekly_review
    DROP CONSTRAINT IF EXISTS weekly_review_commitments_are_tallies,
    DROP COLUMN IF EXISTS commitments_kept,
    DROP COLUMN IF EXISTS commitments_due;

DROP TRIGGER IF EXISTS app_user_forget_weekly_plan_responses ON app_user;
DROP FUNCTION IF EXISTS weekly_plan_commitment_forget_manager();

DROP TABLE IF EXISTS weekly_plan_commitment;
DROP TABLE IF EXISTS weekly_plan;
