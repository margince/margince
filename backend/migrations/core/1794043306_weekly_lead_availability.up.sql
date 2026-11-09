SET LOCAL lock_timeout = '5s';
ALTER TABLE weekly_review ADD COLUMN leads_withheld boolean NOT NULL DEFAULT false;
ALTER TABLE team_weekly_review ADD COLUMN leads_withheld boolean NOT NULL DEFAULT false;
