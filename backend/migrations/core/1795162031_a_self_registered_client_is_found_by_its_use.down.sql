-- pgmigrate:no-transaction
SET lock_timeout = '2s';
DROP INDEX CONCURRENTLY IF EXISTS idx_oauth_client_self_registered;
DROP INDEX CONCURRENTLY IF EXISTS idx_oauth_grant_client;
RESET lock_timeout;
