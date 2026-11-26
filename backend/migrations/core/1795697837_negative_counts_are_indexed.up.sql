-- pgmigrate:no-transaction
-- The rows the negative counts data task corrects. Partial, so each index holds
-- only rows still to correct, and the task never scans the whole history. Each
-- drop clears an index a failed concurrent build left.
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_negative_duration;
CREATE INDEX CONCURRENTLY idx_activity_negative_duration ON activity (id) WHERE duration_seconds < 0;
DROP INDEX CONCURRENTLY IF EXISTS idx_ai_call_negative_tokens;
CREATE INDEX CONCURRENTLY idx_ai_call_negative_tokens ON ai_call (id)
    WHERE tokens_in < 0 OR tokens_out < 0 OR reasoning_tokens < 0 OR cached_tokens < 0 OR cache_write_tokens < 0;
