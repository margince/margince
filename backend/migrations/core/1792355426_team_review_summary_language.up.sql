SET LOCAL lock_timeout = '5s';

ALTER TABLE team_weekly_review ADD COLUMN summary_language text;
