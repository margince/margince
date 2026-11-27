-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The columns come back with their types, defaults and checks; the down step
-- of the previous migration rebuilds their indexes. What they held is not
-- restored: a returning row carries the default or a blank.
SET LOCAL lock_timeout = '3s';

ALTER TABLE vault_secret ADD COLUMN IF NOT EXISTS key_version integer NOT NULL DEFAULT 1;
ALTER TABLE vault_secret ALTER COLUMN key_version DROP DEFAULT;

ALTER TABLE federated_identity ADD COLUMN IF NOT EXISTS email_at_link text NOT NULL DEFAULT '';
ALTER TABLE federated_identity ALTER COLUMN email_at_link DROP DEFAULT;

ALTER TABLE project_phase_history ADD COLUMN IF NOT EXISTS changed_by text NOT NULL DEFAULT '';
ALTER TABLE project_phase_history ALTER COLUMN changed_by DROP DEFAULT;

ALTER TABLE partner
    ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT 'manual'
        CONSTRAINT partner_source_check CHECK ((source = ANY (ARRAY['api'::text, 'automation'::text, 'capture'::text, 'import'::text, 'manual'::text, 'webform'::text])));
ALTER TABLE partner ALTER COLUMN source DROP DEFAULT;

ALTER TABLE report_edition
    ADD COLUMN IF NOT EXISTS report_revision bigint NOT NULL DEFAULT 1
        CONSTRAINT report_edition_report_revision_js_safe CHECK (((report_revision >= '-9007199254740991'::bigint) AND (report_revision <= '9007199254740991'::bigint)));
ALTER TABLE report_edition ALTER COLUMN report_revision DROP DEFAULT;

ALTER TABLE auth_token
    ADD COLUMN IF NOT EXISTS purpose text NOT NULL DEFAULT 'password_reset'
        CONSTRAINT auth_token_purpose_check CHECK ((purpose = ANY (ARRAY['password_reset'::text, 'email_verify'::text, 'invite'::text])));
ALTER TABLE auth_token ALTER COLUMN purpose DROP DEFAULT;

ALTER TABLE finance_connection ADD COLUMN IF NOT EXISTS archived_at timestamptz;

ALTER TABLE voice_learning_signal
    ADD COLUMN IF NOT EXISTS final_text text,
    ADD COLUMN IF NOT EXISTS transformations jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE voice_learning_signal
    ADD COLUMN IF NOT EXISTS qualifies_as_source boolean NOT NULL DEFAULT false
        CONSTRAINT voice_learning_signal_qualifies_check
        CHECK (((NOT qualifies_as_source) OR ((outcome = 'edited_sent'::text) AND (final_text IS NOT NULL) AND (final_captured_by ~~ 'human:%'::text))));

ALTER TABLE activity_review_template ADD COLUMN IF NOT EXISTS archived_at timestamptz;

ALTER TABLE linkedin_connection ADD COLUMN IF NOT EXISTS provider_member_ref text;
