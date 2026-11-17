SET LOCAL lock_timeout = '3s';
-- Until when a refusal and its receipt stay out of every retention policy's
-- reach, stamped from the subject's messaging rules when they are written.
-- Null sets no floor.
ALTER TABLE communication_suppression ADD COLUMN kept_until timestamptz;
ALTER TABLE comms_outbound ADD COLUMN kept_until timestamptz;
