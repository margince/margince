-- Postgres cannot un-validate a constraint, so it is dropped and added back NOT VALID.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_model_rate
    DROP CONSTRAINT ai_model_rate_long_context_complete_check,
    ADD CONSTRAINT ai_model_rate_long_context_complete_check CHECK (
        (long_context_above_tokens IS NULL) = (long_context_input_per_mtok_microusd IS NULL)
        AND (long_context_above_tokens IS NULL) = (long_context_output_per_mtok_microusd IS NULL)
        AND (long_context_above_tokens IS NULL) = (long_context_cache_read_per_mtok_microusd IS NULL)
        AND (long_context_above_tokens IS NULL) = (long_context_cache_write_per_mtok_microusd IS NULL)) NOT VALID;
