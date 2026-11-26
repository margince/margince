-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- No statement, trigger, export or erasure path read these columns. A check
-- that goes with them compared only columns dropped here.
SET LOCAL lock_timeout = '3s';

ALTER TABLE assurance_deal_receipt DROP COLUMN IF EXISTS assessed_at,
    DROP COLUMN IF EXISTS valid_until;

ALTER TABLE chat_post_item DROP COLUMN IF EXISTS attempts,
    DROP COLUMN IF EXISTS entity_id,
    DROP COLUMN IF EXISTS entity_type,
    DROP COLUMN IF EXISTS last_error,
    DROP COLUMN IF EXISTS tier;

ALTER TABLE chat_link DROP COLUMN IF EXISTS channel_external_key,
    DROP COLUMN IF EXISTS history_synced_at;

-- A history pull moves the cursor and leaves the link's version alone.
CREATE OR REPLACE TRIGGER trg_chat_link_updated BEFORE UPDATE ON chat_link FOR EACH ROW
    WHEN ((to_jsonb(OLD) - ARRAY['history_cursor', 'card_message_id', 'card_fingerprint'])
        IS DISTINCT FROM (to_jsonb(NEW) - ARRAY['history_cursor', 'card_message_id', 'card_fingerprint']))
    EXECUTE FUNCTION set_updated_at_bump_version();

ALTER TABLE forecast_snapshot DROP COLUMN IF EXISTS evidence_cutoff;

-- A saved run is read by its question, which is asked again for each reader.
ALTER TABLE report_run DROP COLUMN IF EXISTS result_columns,
    DROP COLUMN IF EXISTS result_rows,
    DROP COLUMN IF EXISTS schema_version,
    DROP COLUMN IF EXISTS total_safe,
    DROP COLUMN IF EXISTS withheld;

ALTER TABLE report_edition_contribution DROP COLUMN IF EXISTS group_key,
    DROP COLUMN IF EXISTS owner_id;

ALTER TABLE list_evaluation DROP COLUMN IF EXISTS definition_version,
    DROP COLUMN IF EXISTS member_count;

ALTER TABLE agent_run DROP COLUMN IF EXISTS steps_used,
    DROP COLUMN IF EXISTS output_tokens;

ALTER TABLE runner_job DROP COLUMN IF EXISTS agent_run_id,
    DROP COLUMN IF EXISTS attempts,
    DROP COLUMN IF EXISTS last_error;

ALTER TABLE agent_standing_grant DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS updated_at;

ALTER TABLE provider_run_reservation DROP COLUMN IF EXISTS reserved_at,
    DROP COLUMN IF EXISTS reconciled_at;

ALTER TABLE bulk_operation DROP COLUMN IF EXISTS passport_id;

ALTER TABLE confirm_token DROP COLUMN IF EXISTS fetch_count,
    DROP COLUMN IF EXISTS first_fetched_at;

-- The pass walks its pending members, so the run stores no position.
ALTER TABLE close_date_run DROP COLUMN IF EXISTS cursor_created_at,
    DROP COLUMN IF EXISTS cursor_id;

ALTER TABLE capture_connection DROP COLUMN IF EXISTS scopes;

ALTER TABLE capture_sender_override DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS updated_at;

DROP TRIGGER IF EXISTS trg_capture_sync_state_updated ON capture_sync_state;
ALTER TABLE capture_sync_state DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS updated_at;

ALTER TABLE capture_alias_refused DROP COLUMN IF EXISTS refused_at;

ALTER TABLE capture_alias_sighting DROP COLUMN IF EXISTS created_at;

ALTER TABLE capture_auto_enrich_state DROP COLUMN IF EXISTS created_at;

ALTER TABLE capture_freemail_domain DROP COLUMN IF EXISTS created_by;

ALTER TABLE capture_pending_counterparty DROP COLUMN IF EXISTS served_model;
