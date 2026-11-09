SET LOCAL lock_timeout = '3s';

ALTER TABLE weekly_review DROP COLUMN IF EXISTS daily_activity;
