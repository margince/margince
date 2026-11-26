-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The columns come back with their types, defaults, checks, links, indexes and
-- touch triggers. What they held is not restored: a returning row carries the
-- default or a blank.
SET LOCAL lock_timeout = '3s';

ALTER TABLE automation
    ADD COLUMN IF NOT EXISTS origin text NOT NULL DEFAULT 'catalog'
        CONSTRAINT automation_origin_check CHECK ((origin = ANY (ARRAY['catalog'::text, 'agent_authored'::text])));

ALTER TABLE tag_suggestion
    ADD COLUMN IF NOT EXISTS evidence_through timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS decided_by uuid
        CONSTRAINT tag_suggestion_decided_by_fkey REFERENCES app_user(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE tag_suggestion ALTER COLUMN evidence_through DROP DEFAULT;
CREATE INDEX IF NOT EXISTS idx_tag_suggestion_decided_by ON tag_suggestion (decided_by)
    WHERE (decided_by IS NOT NULL);
CREATE OR REPLACE TRIGGER trg_tag_suggestion_updated BEFORE UPDATE ON tag_suggestion
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE analytics_share
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 1
        CONSTRAINT analytics_share_version_js_safe CHECK (((version >= '-9007199254740991'::bigint) AND (version <= '9007199254740991'::bigint)));

ALTER TABLE contact_signature_enrich_state
    ADD COLUMN IF NOT EXISTS last_skip_reason text
        CONSTRAINT contact_signature_enrich_state_last_skip_reason_check CHECK ((last_skip_reason = ANY (ARRAY['name_gate'::text, 'evidence_gate'::text, 'confidence'::text, 'narrowed'::text]))),
    ADD COLUMN IF NOT EXISTS last_outcome text
        CONSTRAINT contact_signature_enrich_state_last_outcome_check CHECK ((last_outcome = ANY (ARRAY['applied'::text, 'nothing_to_apply'::text, 'skipped'::text])));

ALTER TABLE finance_connection
    ADD COLUMN IF NOT EXISTS credential_ref text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT '';
ALTER TABLE finance_connection ALTER COLUMN credential_ref DROP DEFAULT,
    ALTER COLUMN source DROP DEFAULT;

ALTER TABLE finance_customer_link
    ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS sync_hash text NOT NULL DEFAULT '';
ALTER TABLE finance_customer_link ALTER COLUMN source DROP DEFAULT,
    ALTER COLUMN sync_hash DROP DEFAULT;

ALTER TABLE finance_external_customer ADD COLUMN IF NOT EXISTS source_updated_at timestamptz;

ALTER TABLE finance_invoice ADD COLUMN IF NOT EXISTS source_updated_at timestamptz;

ALTER TABLE meeting_readiness
    ADD COLUMN IF NOT EXISTS captured_by text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS confirmed_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE meeting_readiness ALTER COLUMN captured_by DROP DEFAULT,
    ALTER COLUMN confirmed_at DROP DEFAULT;

ALTER TABLE weekly_plan ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE weekly_plan_commitment ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE weekly_review_learning ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE weekly_review_scorecard ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE booking_alias ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE company_domain_disposition ADD COLUMN IF NOT EXISTS last_attempt_at timestamptz;

ALTER TABLE company_geocode_state ADD COLUMN IF NOT EXISTS last_outcome text;

ALTER TABLE contact_moment_dismissal ADD COLUMN IF NOT EXISTS dismissed_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE extension_ingest_position ADD COLUMN IF NOT EXISTS saved_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE extension_ingest_refusal ADD COLUMN IF NOT EXISTS first_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE extension_ingest_refusal ALTER COLUMN first_at DROP DEFAULT;

ALTER TABLE graph_contact_edge ADD COLUMN IF NOT EXISTS computed_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE graph_interaction_edge ADD COLUMN IF NOT EXISTS computed_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE knowledge_chunk ADD COLUMN IF NOT EXISTS embedded_at timestamptz;

ALTER TABLE knowledge_corpus ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();
CREATE OR REPLACE TRIGGER knowledge_corpus_touch BEFORE UPDATE ON knowledge_corpus
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE knowledge_document ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();
CREATE OR REPLACE TRIGGER knowledge_document_touch BEFORE UPDATE ON knowledge_document
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE list_live_member
    ADD COLUMN IF NOT EXISTS definition_version bigint NOT NULL DEFAULT 0
        CONSTRAINT list_live_member_definition_version_js_safe CHECK (((definition_version >= '-9007199254740991'::bigint) AND (definition_version <= '9007199254740991'::bigint)));
ALTER TABLE list_live_member ALTER COLUMN definition_version DROP DEFAULT;

ALTER TABLE product_translation ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE restore_drill ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE site_read ADD COLUMN IF NOT EXISTS first_grounded_at timestamptz;

ALTER TABLE tag_interest_read ADD COLUMN IF NOT EXISTS read_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE voice_learning_signal ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT '';
ALTER TABLE voice_learning_signal ALTER COLUMN source DROP DEFAULT;

ALTER TABLE voice_profile ADD COLUMN IF NOT EXISTS model_ref text;

ALTER TABLE worklist_done ADD COLUMN IF NOT EXISTS set_by text NOT NULL DEFAULT '';
ALTER TABLE worklist_done ALTER COLUMN set_by DROP DEFAULT;

ALTER TABLE worklist_pin ADD COLUMN IF NOT EXISTS set_by text NOT NULL DEFAULT '';
ALTER TABLE worklist_pin ALTER COLUMN set_by DROP DEFAULT;
