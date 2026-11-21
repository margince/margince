-- pgmigrate:no-transaction
SET lock_timeout = '2s';
DROP INDEX CONCURRENTLY IF EXISTS idx_deal_live_newest;
DROP INDEX CONCURRENTLY IF EXISTS idx_webhook_delivery_pending;
RESET lock_timeout;
