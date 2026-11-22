-- pgmigrate:no-transaction
SET lock_timeout = '2s';
DROP INDEX CONCURRENTLY IF EXISTS idx_aparticipant_one_roster_seat;
RESET lock_timeout;
