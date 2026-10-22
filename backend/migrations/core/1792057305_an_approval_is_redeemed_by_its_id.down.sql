-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

CREATE TABLE signing_key (
    kid text NOT NULL,
    private_key bytea NOT NULL,
    public_key bytea NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    retired_at timestamptz,
    CONSTRAINT signing_key_pkey PRIMARY KEY (kid));
GRANT SELECT, INSERT, DELETE, UPDATE ON TABLE signing_key TO margince_app;

COMMENT ON COLUMN approval.diff_hash IS
	'Hash of the staged proposed_change; the minted approval_token is bound to this hash so a token cannot authorize a different effect.';
