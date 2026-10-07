SET LOCAL lock_timeout = '3s';
ALTER TABLE weekly_review ADD COLUMN numeric_summary jsonb;
ALTER TABLE team_weekly_review ADD COLUMN numeric_summary jsonb;
