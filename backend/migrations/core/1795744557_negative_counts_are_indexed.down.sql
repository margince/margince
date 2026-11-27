-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_ai_call_negative_tokens;
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_negative_duration;
