-- Same bound as the up: dropping a column takes an ACCESS EXCLUSIVE lock on a
-- table the live product writes every morning.
SET LOCAL lock_timeout = '3s';

ALTER TABLE brief_run DROP CONSTRAINT IF EXISTS brief_run_mail_error_length;
ALTER TABLE brief_run DROP CONSTRAINT IF EXISTS brief_run_mail_error_needs_an_attempt;
ALTER TABLE brief_run DROP COLUMN IF EXISTS mail_error;
ALTER TABLE brief_run DROP COLUMN IF EXISTS mail_attempted_at;
