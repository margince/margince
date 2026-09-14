SET LOCAL lock_timeout = '5s';

-- NULL preserves the distinction between older reports and a measured zero.
ALTER TABLE weekly_review ADD COLUMN tasks_completed integer
    CHECK (tasks_completed >= 0);
