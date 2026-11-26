-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- No statement, trigger, export or erasure path read these columns. A check,
-- link, index or touch trigger that goes with them named only a dropped column.
SET LOCAL lock_timeout = '3s';

ALTER TABLE automation DROP COLUMN IF EXISTS origin;

DROP TRIGGER IF EXISTS trg_tag_suggestion_updated ON tag_suggestion;
ALTER TABLE tag_suggestion DROP COLUMN IF EXISTS evidence_through,
    DROP COLUMN IF EXISTS decided_by,
    DROP COLUMN IF EXISTS updated_at;

ALTER TABLE analytics_share DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS version;

ALTER TABLE contact_signature_enrich_state DROP COLUMN IF EXISTS last_skip_reason,
    DROP COLUMN IF EXISTS last_outcome;

ALTER TABLE finance_connection DROP COLUMN IF EXISTS credential_ref,
    DROP COLUMN IF EXISTS source;

ALTER TABLE finance_customer_link DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS sync_hash;

ALTER TABLE finance_external_customer DROP COLUMN IF EXISTS source_updated_at;

ALTER TABLE finance_invoice DROP COLUMN IF EXISTS source_updated_at;

ALTER TABLE meeting_readiness DROP COLUMN IF EXISTS captured_by,
    DROP COLUMN IF EXISTS confirmed_at;

ALTER TABLE weekly_plan DROP COLUMN IF EXISTS updated_at;

ALTER TABLE weekly_plan_commitment DROP COLUMN IF EXISTS updated_at;

ALTER TABLE weekly_review_learning DROP COLUMN IF EXISTS created_at;

ALTER TABLE weekly_review_scorecard DROP COLUMN IF EXISTS created_at;

ALTER TABLE booking_alias DROP COLUMN IF EXISTS created_at;

ALTER TABLE company_domain_disposition DROP COLUMN IF EXISTS last_attempt_at;

ALTER TABLE company_geocode_state DROP COLUMN IF EXISTS last_outcome;

ALTER TABLE contact_moment_dismissal DROP COLUMN IF EXISTS dismissed_at;

ALTER TABLE extension_ingest_position DROP COLUMN IF EXISTS saved_at;

ALTER TABLE extension_ingest_refusal DROP COLUMN IF EXISTS first_at;

ALTER TABLE graph_contact_edge DROP COLUMN IF EXISTS computed_at;

ALTER TABLE graph_interaction_edge DROP COLUMN IF EXISTS computed_at;

ALTER TABLE knowledge_chunk DROP COLUMN IF EXISTS embedded_at;

DROP TRIGGER IF EXISTS knowledge_corpus_touch ON knowledge_corpus;
ALTER TABLE knowledge_corpus DROP COLUMN IF EXISTS updated_at;

DROP TRIGGER IF EXISTS knowledge_document_touch ON knowledge_document;
ALTER TABLE knowledge_document DROP COLUMN IF EXISTS updated_at;

ALTER TABLE list_live_member DROP COLUMN IF EXISTS definition_version;

ALTER TABLE product_translation DROP COLUMN IF EXISTS created_at;

ALTER TABLE restore_drill DROP COLUMN IF EXISTS created_at;

ALTER TABLE site_read DROP COLUMN IF EXISTS first_grounded_at;

ALTER TABLE tag_interest_read DROP COLUMN IF EXISTS read_at;

ALTER TABLE voice_learning_signal DROP COLUMN IF EXISTS source;

ALTER TABLE voice_profile DROP COLUMN IF EXISTS model_ref;

ALTER TABLE worklist_done DROP COLUMN IF EXISTS set_by;

ALTER TABLE worklist_pin DROP COLUMN IF EXISTS set_by;
