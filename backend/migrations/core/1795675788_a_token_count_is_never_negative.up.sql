-- A token counter counts what a provider spent, so it is never below zero.
-- The meters store a negative provider count as zero and log it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_usage
	ADD CONSTRAINT ai_usage_calls_not_negative CHECK (calls >= 0) NOT VALID;
ALTER TABLE ai_usage
	ADD CONSTRAINT ai_usage_cached_hits_not_negative CHECK (cached_hits >= 0) NOT VALID;
ALTER TABLE ai_usage
	ADD CONSTRAINT ai_usage_tokens_in_not_negative CHECK (tokens_in >= 0) NOT VALID;
ALTER TABLE ai_usage
	ADD CONSTRAINT ai_usage_tokens_out_not_negative CHECK (tokens_out >= 0) NOT VALID;
ALTER TABLE ai_call
	ADD CONSTRAINT ai_call_tokens_in_not_negative CHECK (tokens_in >= 0) NOT VALID;
ALTER TABLE ai_call
	ADD CONSTRAINT ai_call_tokens_out_not_negative CHECK (tokens_out >= 0) NOT VALID;
ALTER TABLE ai_call
	ADD CONSTRAINT ai_call_reasoning_tokens_not_negative CHECK (reasoning_tokens >= 0) NOT VALID;
ALTER TABLE ai_call
	ADD CONSTRAINT ai_call_cached_tokens_not_negative CHECK (cached_tokens >= 0) NOT VALID;
ALTER TABLE ai_call
	ADD CONSTRAINT ai_call_cache_write_tokens_not_negative CHECK (cache_write_tokens >= 0) NOT VALID;
