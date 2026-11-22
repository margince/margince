-- pgmigrate:no-transaction
SET lock_timeout = '2s';
-- The data task that moves roster_seat into roster_seats pages over these rows.
-- Few rows hold a seat, so without the index a batch reads the whole table to find them.
DROP INDEX CONCURRENTLY IF EXISTS idx_aparticipant_one_roster_seat;
CREATE INDEX CONCURRENTLY idx_aparticipant_one_roster_seat ON activity_participant (id) WHERE roster_seat IS NOT NULL;
RESET lock_timeout;
