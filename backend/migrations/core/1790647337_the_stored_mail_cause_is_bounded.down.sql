-- Reverses 1790647337.
SET LOCAL lock_timeout = '3s';
ALTER TABLE notice DROP CONSTRAINT notice_email_error_bounded;
