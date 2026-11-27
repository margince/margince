SET LOCAL lock_timeout = '5s';
DROP TRIGGER IF EXISTS activity_clears_negative_duration ON activity;
DROP FUNCTION IF EXISTS activity_clears_negative_duration();
ALTER TABLE activity DROP CONSTRAINT IF EXISTS activity_duration_not_negative;
