-- pgmigrate:no-transaction
SET lock_timeout = '3s';

DROP INDEX CONCURRENTLY IF EXISTS idx_own_domain_spelling_cursor;

RESET lock_timeout;
