-- Same bound as the up: dropping a column takes an ACCESS EXCLUSIVE lock on a
-- table the live product writes every Monday.
SET LOCAL lock_timeout = '3s';

ALTER TABLE weekly_review DROP CONSTRAINT IF EXISTS weekly_review_mail_error_length;
ALTER TABLE weekly_review DROP CONSTRAINT IF EXISTS weekly_review_mail_error_needs_an_attempt;
ALTER TABLE weekly_review DROP COLUMN IF EXISTS mail_error;
ALTER TABLE weekly_review DROP COLUMN IF EXISTS mail_attempted_at;
