-- Validates the floors the previous migration added. A file of its own, so
-- writers pass while each table is scanned under the lighter lock.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_usage VALIDATE CONSTRAINT ai_usage_calls_not_negative;
ALTER TABLE ai_usage VALIDATE CONSTRAINT ai_usage_cached_hits_not_negative;
ALTER TABLE ai_usage VALIDATE CONSTRAINT ai_usage_tokens_in_not_negative;
ALTER TABLE ai_usage VALIDATE CONSTRAINT ai_usage_tokens_out_not_negative;
ALTER TABLE ai_call VALIDATE CONSTRAINT ai_call_tokens_in_not_negative;
ALTER TABLE ai_call VALIDATE CONSTRAINT ai_call_tokens_out_not_negative;
ALTER TABLE ai_call VALIDATE CONSTRAINT ai_call_reasoning_tokens_not_negative;
ALTER TABLE ai_call VALIDATE CONSTRAINT ai_call_cached_tokens_not_negative;
ALTER TABLE ai_call VALIDATE CONSTRAINT ai_call_cache_write_tokens_not_negative;
ALTER TABLE activity VALIDATE CONSTRAINT activity_duration_seconds_not_negative;
