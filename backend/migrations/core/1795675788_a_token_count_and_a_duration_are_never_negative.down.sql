-- Drops the floors this pair added.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_usage DROP CONSTRAINT ai_usage_calls_not_negative;
ALTER TABLE ai_usage DROP CONSTRAINT ai_usage_cached_hits_not_negative;
ALTER TABLE ai_usage DROP CONSTRAINT ai_usage_tokens_in_not_negative;
ALTER TABLE ai_usage DROP CONSTRAINT ai_usage_tokens_out_not_negative;
ALTER TABLE ai_call DROP CONSTRAINT ai_call_tokens_in_not_negative;
ALTER TABLE ai_call DROP CONSTRAINT ai_call_tokens_out_not_negative;
ALTER TABLE ai_call DROP CONSTRAINT ai_call_reasoning_tokens_not_negative;
ALTER TABLE ai_call DROP CONSTRAINT ai_call_cached_tokens_not_negative;
ALTER TABLE ai_call DROP CONSTRAINT ai_call_cache_write_tokens_not_negative;
ALTER TABLE activity DROP CONSTRAINT activity_duration_seconds_not_negative;
