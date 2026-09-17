SET LOCAL lock_timeout = '3s';
ALTER TABLE notice
  DROP CONSTRAINT notice_email_error_after_attempt,
  DROP COLUMN email_error,
  DROP COLUMN email_attempted_at;
DROP TABLE notification_digest_run;
DROP TABLE notification_preference;
