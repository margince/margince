-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The columns come back with their types, defaults and own checks. What they
-- held is not restored: a returning row carries the default or a blank.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call
    ADD COLUMN IF NOT EXISTS cache_off boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS context_bytes bigint NOT NULL DEFAULT 0
        CONSTRAINT ai_call_context_bytes_check CHECK ((context_bytes >= 0)),
    ADD COLUMN IF NOT EXISTS context_tokens_estimate bigint NOT NULL DEFAULT 0
        CONSTRAINT ai_call_context_tokens_estimate_check CHECK ((context_tokens_estimate >= 0)),
    ADD COLUMN IF NOT EXISTS request_fingerprint text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS schema_downgrade text NOT NULL DEFAULT ''
        CONSTRAINT ai_call_schema_downgrade_check CHECK ((schema_downgrade = ANY (ARRAY[''::text, 'relaxed'::text, 'unenforced'::text, 'dropped'::text]))),
    ADD COLUMN IF NOT EXISTS secret_kinds text[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS secrets_removed bigint NOT NULL DEFAULT 0
        CONSTRAINT ai_call_secrets_removed_check CHECK ((secrets_removed >= 0)),
    ADD COLUMN IF NOT EXISTS finish_reason text NOT NULL DEFAULT '';
ALTER TABLE ai_call ALTER COLUMN request_fingerprint DROP DEFAULT;

ALTER TABLE ai_feedback ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT 'human';
ALTER TABLE ai_feedback ALTER COLUMN source DROP DEFAULT;

ALTER TABLE assurance_deal_receipt
    ADD COLUMN IF NOT EXISTS verdict_prompt_version text,
    ADD COLUMN IF NOT EXISTS captured_by text NOT NULL DEFAULT 'system',
    ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS reasons text[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS silent_days integer NOT NULL DEFAULT 0;
ALTER TABLE assurance_deal_receipt ALTER COLUMN captured_by DROP DEFAULT,
    ALTER COLUMN silent_days DROP DEFAULT;

ALTER TABLE assurance_cycle
    ADD COLUMN IF NOT EXISTS opened_by text NOT NULL DEFAULT 'system',
    ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE assurance_cycle ALTER COLUMN opened_by DROP DEFAULT;

ALTER TABLE assurance_run
    ADD COLUMN IF NOT EXISTS rule_versions jsonb NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS requested_by text
        CONSTRAINT assurance_run_requester_is_named CHECK (((requested_by IS NULL) OR (length(btrim(requested_by)) > 0)));

ALTER TABLE assurance_exception
    ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS episode integer NOT NULL DEFAULT 1;

ALTER TABLE assurance_input_change
    ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE assurance_source_coverage
    ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE assurance_run_finding ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();
