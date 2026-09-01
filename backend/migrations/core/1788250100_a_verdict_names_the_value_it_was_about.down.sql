SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_feedback
  DROP COLUMN value_captured_at,
  DROP COLUMN value_shown;
