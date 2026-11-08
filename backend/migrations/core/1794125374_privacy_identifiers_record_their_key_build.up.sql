-- pgmigrate:no-transaction
-- An old preference link is found by its hash, which is unique like the link.
-- The other three hold only the rows still to re-key or prune, so each scan
-- shrinks as its work is done. Each first drop clears an invalid index a
-- failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS preference_token_token_hash_key;
CREATE UNIQUE INDEX CONCURRENTLY preference_token_token_hash_key
    ON preference_token (token_hash) WHERE token_hash IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_webhook_delivery_body_kept;
CREATE INDEX CONCURRENTLY idx_webhook_delivery_body_kept
    ON webhook_delivery (id)
    WHERE payload_pruned_at IS NULL AND payload <> ''
      AND status IN ('delivered', 'dead_lettered', 'visibility_revoked');
DROP INDEX CONCURRENTLY IF EXISTS idx_system_log_unkeyed_email;
CREATE INDEX CONCURRENTLY idx_system_log_unkeyed_email
    ON system_log (id)
    WHERE action = 'login' AND detail ? 'email_hash' AND NOT detail ? 'email_key_id';
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_unkeyed_address_hashes;
CREATE INDEX CONCURRENTLY idx_contact_unkeyed_address_hashes
    ON contact (id)
    WHERE anonymized_address_hashes IS NOT NULL AND anonymized_address_key_id IS NULL;
