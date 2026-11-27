-- pgmigrate:no-transaction
-- The index behind the room branch of search, built while rooms are still
-- written.
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS idx_deal_room_search;
CREATE INDEX CONCURRENTLY idx_deal_room_search ON deal_room USING gin (search_tsv);
RESET lock_timeout;
