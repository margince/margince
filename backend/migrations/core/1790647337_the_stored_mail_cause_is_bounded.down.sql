-- Reverses 1790584337.
SET LOCAL lock_timeout = '3s';
ALTER TABLE notice DROP CONSTRAINT notice_email_error_bounded;
