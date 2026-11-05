-- Some models charge more for every token of a call whose prompt passes a size.
-- gemini-3.1-pro-preview costs $2/$12 per MTok up to 200k prompt tokens and
-- $4/$18 above it. A rate row may now carry that second band: the prompt size
-- it starts above and the four prices that apply there. The columns stay empty
-- for a model with one flat price, which is every model but one today. A band
-- is all five columns or none, so a half-written band cannot price a call.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_model_rate
    ADD COLUMN long_context_above_tokens integer
        CONSTRAINT ai_model_rate_long_context_above_tokens_check CHECK (long_context_above_tokens > 0),
    ADD COLUMN long_context_input_per_mtok_microusd bigint
        CONSTRAINT ai_model_rate_long_context_input_check CHECK (long_context_input_per_mtok_microusd >= 0),
    ADD COLUMN long_context_output_per_mtok_microusd bigint
        CONSTRAINT ai_model_rate_long_context_output_check CHECK (long_context_output_per_mtok_microusd >= 0),
    ADD COLUMN long_context_cache_read_per_mtok_microusd bigint
        CONSTRAINT ai_model_rate_long_context_cache_read_check CHECK (long_context_cache_read_per_mtok_microusd >= 0),
    ADD COLUMN long_context_cache_write_per_mtok_microusd bigint
        CONSTRAINT ai_model_rate_long_context_cache_write_check CHECK (long_context_cache_write_per_mtok_microusd >= 0),
    ADD CONSTRAINT ai_model_rate_long_context_complete_check CHECK (
        (long_context_above_tokens IS NULL) = (long_context_input_per_mtok_microusd IS NULL)
        AND (long_context_above_tokens IS NULL) = (long_context_output_per_mtok_microusd IS NULL)
        AND (long_context_above_tokens IS NULL) = (long_context_cache_read_per_mtok_microusd IS NULL)
        AND (long_context_above_tokens IS NULL) = (long_context_cache_write_per_mtok_microusd IS NULL)) NOT VALID;
