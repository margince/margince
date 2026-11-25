-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The columns come back with their defaults. The counts that were dropped are
-- not restored: ai_call still holds them per call. A returning run names the
-- nil event, because the event that last wrote it was not kept.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_usage
    ADD COLUMN IF NOT EXISTS cache_write_tokens bigint NOT NULL DEFAULT 0
        CONSTRAINT ai_usage_cache_write_tokens_js_safe CHECK (((cache_write_tokens >= '-9007199254740991'::bigint) AND (cache_write_tokens <= '9007199254740991'::bigint))),
    ADD COLUMN IF NOT EXISTS cached_tokens bigint NOT NULL DEFAULT 0
        CONSTRAINT ai_usage_cached_tokens_js_safe CHECK (((cached_tokens >= '-9007199254740991'::bigint) AND (cached_tokens <= '9007199254740991'::bigint))),
    ADD COLUMN IF NOT EXISTS reasoning_tokens bigint NOT NULL DEFAULT 0
        CONSTRAINT ai_usage_reasoning_tokens_js_safe CHECK (((reasoning_tokens >= '-9007199254740991'::bigint) AND (reasoning_tokens <= '9007199254740991'::bigint)));

ALTER TABLE ai_task_run
    ADD COLUMN IF NOT EXISTS last_event_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE ai_task_run ALTER COLUMN last_event_id DROP DEFAULT;

CREATE TRIGGER trg_ai_task_run_updated BEFORE UPDATE ON ai_task_run
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
