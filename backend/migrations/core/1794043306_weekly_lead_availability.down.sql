SET LOCAL lock_timeout = '5s';
ALTER TABLE team_weekly_review DROP COLUMN leads_withheld;
ALTER TABLE weekly_review DROP COLUMN leads_withheld;
