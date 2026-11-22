-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- No statement, trigger or cross-column check read these columns. The price,
-- the trace and the health reads of ai_call use its other columns.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call DROP COLUMN IF EXISTS cache_off,
    DROP COLUMN IF EXISTS context_bytes,
    DROP COLUMN IF EXISTS context_tokens_estimate,
    DROP COLUMN IF EXISTS request_fingerprint,
    DROP COLUMN IF EXISTS schema_downgrade,
    DROP COLUMN IF EXISTS secret_kinds,
    DROP COLUMN IF EXISTS secrets_removed,
    DROP COLUMN IF EXISTS finish_reason;

ALTER TABLE ai_feedback DROP COLUMN IF EXISTS source;

-- The input check reads a receipt by its outcome, policy, run and fingerprint.
ALTER TABLE assurance_deal_receipt DROP COLUMN IF EXISTS verdict_prompt_version,
    DROP COLUMN IF EXISTS captured_by,
    DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS reasons,
    DROP COLUMN IF EXISTS silent_days;

ALTER TABLE assurance_cycle DROP COLUMN IF EXISTS opened_by,
    DROP COLUMN IF EXISTS created_at;

ALTER TABLE assurance_run DROP COLUMN IF EXISTS rule_versions,
    DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS requested_by;

ALTER TABLE assurance_exception DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS episode;

ALTER TABLE assurance_input_change DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS updated_at;

ALTER TABLE assurance_source_coverage DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS updated_at;

ALTER TABLE assurance_run_finding DROP COLUMN IF EXISTS created_at;
