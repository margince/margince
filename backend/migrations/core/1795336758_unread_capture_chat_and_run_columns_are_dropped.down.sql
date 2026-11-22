-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The columns come back with their types, defaults, checks and links. What
-- they held is not restored: a returning row carries the default or a blank.
SET LOCAL lock_timeout = '3s';

ALTER TABLE assurance_deal_receipt
    ADD COLUMN IF NOT EXISTS assessed_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS valid_until timestamptz NOT NULL DEFAULT now()
        CONSTRAINT assurance_deal_receipt_valid_after_assessed CHECK ((valid_until >= assessed_at));
ALTER TABLE assurance_deal_receipt ALTER COLUMN assessed_at DROP DEFAULT,
    ALTER COLUMN valid_until DROP DEFAULT;

ALTER TABLE chat_post_item
    ADD COLUMN IF NOT EXISTS attempts integer NOT NULL DEFAULT 0
        CONSTRAINT chat_post_item_attempts_check CHECK ((attempts >= 0)),
    ADD COLUMN IF NOT EXISTS entity_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    ADD COLUMN IF NOT EXISTS entity_type text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_error text,
    ADD COLUMN IF NOT EXISTS tier text NOT NULL DEFAULT 'everything'
        CONSTRAINT chat_post_item_tier_check CHECK ((tier = ANY (ARRAY['milestone'::text, 'important'::text, 'everything'::text])));
ALTER TABLE chat_post_item ALTER COLUMN entity_id DROP DEFAULT,
    ALTER COLUMN entity_type DROP DEFAULT,
    ALTER COLUMN tier DROP DEFAULT;

ALTER TABLE chat_link
    ADD COLUMN IF NOT EXISTS channel_external_key text,
    ADD COLUMN IF NOT EXISTS history_synced_at timestamptz;

CREATE OR REPLACE TRIGGER trg_chat_link_updated BEFORE UPDATE ON chat_link FOR EACH ROW
    WHEN ((to_jsonb(OLD) - ARRAY['history_cursor', 'history_synced_at', 'card_message_id', 'card_fingerprint'])
        IS DISTINCT FROM (to_jsonb(NEW) - ARRAY['history_cursor', 'history_synced_at', 'card_message_id', 'card_fingerprint']))
    EXECUTE FUNCTION set_updated_at_bump_version();

ALTER TABLE forecast_snapshot ADD COLUMN IF NOT EXISTS evidence_cutoff timestamptz;

ALTER TABLE report_run
    ADD COLUMN IF NOT EXISTS result_columns jsonb NOT NULL DEFAULT '[]'
        CONSTRAINT report_run_result_columns_is_a_list CHECK ((jsonb_typeof(result_columns) = 'array'::text)),
    ADD COLUMN IF NOT EXISTS result_rows jsonb NOT NULL DEFAULT '[]'
        CONSTRAINT report_run_result_rows_is_a_list CHECK ((jsonb_typeof(result_rows) = 'array'::text)),
    ADD COLUMN IF NOT EXISTS schema_version text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS total_safe boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS withheld boolean NOT NULL DEFAULT false;
ALTER TABLE report_run ALTER COLUMN result_columns DROP DEFAULT,
    ALTER COLUMN result_rows DROP DEFAULT,
    ALTER COLUMN schema_version DROP DEFAULT,
    ALTER COLUMN total_safe DROP DEFAULT,
    ALTER COLUMN withheld DROP DEFAULT;

ALTER TABLE report_edition_contribution
    ADD COLUMN IF NOT EXISTS group_key text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS owner_id uuid;
ALTER TABLE report_edition_contribution ALTER COLUMN group_key DROP DEFAULT;

ALTER TABLE list_evaluation
    ADD COLUMN IF NOT EXISTS definition_version bigint NOT NULL DEFAULT 0
        CONSTRAINT list_evaluation_definition_version_js_safe CHECK (((definition_version >= '-9007199254740991'::bigint) AND (definition_version <= '9007199254740991'::bigint))),
    ADD COLUMN IF NOT EXISTS member_count integer NOT NULL DEFAULT 0;
ALTER TABLE list_evaluation ALTER COLUMN definition_version DROP DEFAULT,
    ALTER COLUMN member_count DROP DEFAULT;

ALTER TABLE agent_run
    ADD COLUMN IF NOT EXISTS steps_used integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS output_tokens integer NOT NULL DEFAULT 0;

ALTER TABLE runner_job
    ADD COLUMN IF NOT EXISTS agent_run_id uuid
        CONSTRAINT runner_job_run_fkey REFERENCES agent_run(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS attempts integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_error text;

ALTER TABLE agent_standing_grant
    ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE provider_run_reservation
    ADD COLUMN IF NOT EXISTS reserved_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS reconciled_at timestamptz;

ALTER TABLE bulk_operation ADD COLUMN IF NOT EXISTS passport_id uuid;

ALTER TABLE confirm_token
    ADD COLUMN IF NOT EXISTS fetch_count integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS first_fetched_at timestamptz;

ALTER TABLE close_date_run
    ADD COLUMN IF NOT EXISTS cursor_created_at timestamptz,
    ADD COLUMN IF NOT EXISTS cursor_id uuid
        CONSTRAINT close_date_run_cursor_is_whole CHECK (((cursor_created_at IS NULL) = (cursor_id IS NULL)));

ALTER TABLE capture_connection ADD COLUMN IF NOT EXISTS scopes text[] NOT NULL DEFAULT '{}';

ALTER TABLE capture_sender_override
    ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE capture_sync_state
    ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();
CREATE OR REPLACE TRIGGER trg_capture_sync_state_updated BEFORE UPDATE ON capture_sync_state
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE capture_alias_refused ADD COLUMN IF NOT EXISTS refused_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE capture_alias_sighting ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE capture_auto_enrich_state ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE capture_freemail_domain
    ADD COLUMN IF NOT EXISTS created_by uuid
        CONSTRAINT capture_freemail_domain_created_by_fkey REFERENCES app_user(id) ON DELETE SET NULL;

ALTER TABLE capture_pending_counterparty ADD COLUMN IF NOT EXISTS served_model text;
