-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- ai_usage drops its three itemized token counts. No statement read them: each
-- call's own row in ai_call holds the same counts, and every price is read there.
-- ai_task_run drops last_event_id, created_at and updated_at. No statement read
-- them either: the feed orders by queued_at, finished_at and seq.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_usage DROP COLUMN IF EXISTS cache_write_tokens,
    DROP COLUMN IF EXISTS cached_tokens,
    DROP COLUMN IF EXISTS reasoning_tokens;

DROP TRIGGER IF EXISTS trg_ai_task_run_updated ON ai_task_run;
ALTER TABLE ai_task_run DROP COLUMN IF EXISTS last_event_id,
    DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS updated_at;
