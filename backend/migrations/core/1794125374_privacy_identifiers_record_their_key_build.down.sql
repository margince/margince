-- pgmigrate:no-transaction
-- Drops the indexes the re-key and the body prune scan.
DROP INDEX CONCURRENTLY IF EXISTS preference_token_token_hash_key;
DROP INDEX CONCURRENTLY IF EXISTS idx_webhook_delivery_body_kept;
DROP INDEX CONCURRENTLY IF EXISTS idx_system_log_unkeyed_email;
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_unkeyed_address_hashes;
