SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_model_rate
    DROP CONSTRAINT IF EXISTS ai_model_rate_long_context_complete_check,
    DROP COLUMN IF EXISTS long_context_above_tokens,
    DROP COLUMN IF EXISTS long_context_input_per_mtok_microusd,
    DROP COLUMN IF EXISTS long_context_output_per_mtok_microusd,
    DROP COLUMN IF EXISTS long_context_cache_read_per_mtok_microusd,
    DROP COLUMN IF EXISTS long_context_cache_write_per_mtok_microusd;
