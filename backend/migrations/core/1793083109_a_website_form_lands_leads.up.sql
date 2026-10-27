-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A website form: one public intake address that turns a form submission into
-- a lead. The key is the address and is public. A secret, where the sender
-- signs, lives in the key vault and only its ref is kept here. No submission is
-- stored: what a visitor sent lands on the lead and its timeline, which the
-- erasure and access paths already reach.
SET LOCAL lock_timeout = '3s';

CREATE TABLE web_form (
    id                     uuid PRIMARY KEY DEFAULT uuidv7(),
    name                   text NOT NULL,
    form_key               text NOT NULL UNIQUE,
    sender                 text NOT NULL,
    allowed_origins        text[] NOT NULL DEFAULT '{}',
    secret_ref             text,
    lead_origin            text REFERENCES lead_origin (key) ON UPDATE CASCADE ON DELETE SET NULL,
    owner_id               uuid REFERENCES app_user (id) ON DELETE SET NULL,
    field_map              jsonb NOT NULL DEFAULT '{}'::jsonb,
    consent_required       boolean NOT NULL DEFAULT false,
    consent_policy_version text,
    consent_wording        text,
    redirect_url           text,
    success_message        text,
    state                  text NOT NULL DEFAULT 'active',
    captured_by            text NOT NULL,
    version                bigint NOT NULL DEFAULT 1,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    archived_at            timestamptz,
    CONSTRAINT web_form_name_present CHECK (length(btrim(name)) > 0 AND length(name) <= 200),
    CONSTRAINT web_form_key_shape CHECK (form_key ~ '^[A-Za-z0-9_-]{16,64}$'),
    CONSTRAINT web_form_sender_check CHECK (sender IN ('browser', 'server', 'webflow', 'typeform', 'tally')),
    CONSTRAINT web_form_state_check CHECK (state IN ('active', 'paused')),
    -- A browser form is admitted by where it was posted from, every other
    -- sender by a signature; each needs its own half and never the other's.
    CONSTRAINT web_form_admission CHECK (
        (sender = 'browser' AND secret_ref IS NULL AND cardinality(allowed_origins) > 0)
        OR (sender <> 'browser' AND secret_ref IS NOT NULL AND cardinality(allowed_origins) = 0)),
    CONSTRAINT web_form_field_map_object CHECK (jsonb_typeof(field_map) = 'object'),
    CONSTRAINT web_form_consent_wording_bounded CHECK (consent_wording IS NULL OR length(consent_wording) <= 2000),
    CONSTRAINT web_form_redirect_https CHECK (redirect_url IS NULL OR redirect_url ~ '^https://'),
    CONSTRAINT web_form_success_message_bounded CHECK (success_message IS NULL OR length(success_message) <= 500),
    CONSTRAINT web_form_version_js_safe
        CHECK (version BETWEEN -9007199254740991 AND 9007199254740991)
);

CREATE INDEX idx_web_form_owner ON web_form (owner_id) WHERE owner_id IS NOT NULL;

-- Last, because a statement after CREATE TRIGGER reads as a rewrite of every table.
CREATE TRIGGER trg_web_form_updated
    BEFORE UPDATE ON web_form
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();
