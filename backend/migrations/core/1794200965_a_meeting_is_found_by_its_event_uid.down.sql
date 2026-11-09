-- pgmigrate:no-transaction
-- Drops the index a meeting is found by its event UID through.
DROP INDEX CONCURRENTLY IF EXISTS activity_identity_meeting_uid;
