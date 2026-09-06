SET LOCAL lock_timeout = '3s';

ALTER TABLE weekly_plan
    DROP CONSTRAINT IF EXISTS weekly_plan_capacity_note_bounded,
    DROP CONSTRAINT IF EXISTS weekly_plan_risks_bounded;

ALTER TABLE weekly_plan
    DROP COLUMN IF EXISTS capacity_note,
    DROP COLUMN IF EXISTS risks;
