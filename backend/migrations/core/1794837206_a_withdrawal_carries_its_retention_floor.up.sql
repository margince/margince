SET LOCAL lock_timeout = '3s';
-- Until when a withdrawal of advertising stays out of every retention policy's
-- reach, set from the subject's messaging rules when the proof row is written.
-- It holds with no stop and no receipt beside it. Null sets no floor.
ALTER TABLE consent_event ADD COLUMN kept_until timestamptz;
