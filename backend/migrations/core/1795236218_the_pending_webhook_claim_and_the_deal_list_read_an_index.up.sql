-- pgmigrate:no-transaction
SET lock_timeout = '2s';
-- The webhook sweep claims a pending delivery older than its lease, and the
-- deal list pages live deals newest first. Without these the claim scans
-- every delivery and each list page sorts every live deal.
-- Built concurrently. Each drop clears an invalid index a failed build left.
DROP INDEX CONCURRENTLY IF EXISTS idx_webhook_delivery_pending;
CREATE INDEX CONCURRENTLY idx_webhook_delivery_pending ON webhook_delivery (created_at)
  WHERE status = 'pending';
DROP INDEX CONCURRENTLY IF EXISTS idx_deal_live_newest;
CREATE INDEX CONCURRENTLY idx_deal_live_newest ON deal (created_at DESC, id DESC)
  WHERE archived_at IS NULL;
RESET lock_timeout;
