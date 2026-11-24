-- pgmigrate:no-transaction
SET lock_timeout = '2s';
-- Whether a consent ever used a client is read by client id, and the clients
-- that registered themselves are read oldest first, under the lock a
-- registration takes. Without these each read walks the whole table.
-- Built concurrently. Each drop clears an invalid index a failed build left.
DROP INDEX CONCURRENTLY IF EXISTS idx_oauth_grant_client;
CREATE INDEX CONCURRENTLY idx_oauth_grant_client ON oauth_grant (client_id);
DROP INDEX CONCURRENTLY IF EXISTS idx_oauth_client_self_registered;
CREATE INDEX CONCURRENTLY idx_oauth_client_self_registered ON oauth_client (created_at, id)
  WHERE created_via IN ('dcr', 'cimd');
RESET lock_timeout;
