-- pgmigrate:no-transaction
-- Finds the merge confirmations that name an account, so deleting that account
-- does not read every token. Built while the app still writes, because
-- auth_token is live. Only confirmation rows carry a counterpart.
DROP INDEX CONCURRENTLY IF EXISTS idx_auth_token_counterpart;
CREATE INDEX CONCURRENTLY idx_auth_token_counterpart ON auth_token (counterpart_user_id)
    WHERE counterpart_user_id IS NOT NULL;
