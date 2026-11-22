-- pgmigrate:no-transaction
SET lock_timeout = '2s';
DROP INDEX CONCURRENTLY IF EXISTS uq_activity_participant_identity;
RESET lock_timeout;
