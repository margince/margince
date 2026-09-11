-- 1789018391: mfa_totp_enrolment_and_recovery_codes.
--
-- The second factor for the password path: a per-user TOTP enrolment and the
-- one-time recovery codes that get a member back in when their authenticator is
-- gone. Both hang off app_user and cascade with it — an account that is deleted
-- has no second factor to keep.

-- One TOTP enrolment per member. The row exists from the moment enrolment
-- STARTS (the secret is generated and sealed) and confirmed_at stays NULL until
-- the member proves possession by entering a first code — an unconfirmed row is
-- a pending enrolment, not an active factor, so a login challenge must ignore
-- it. The secret itself is NEVER stored here: secret_ref is the keyvault handle
-- to the sealed material, so a dump of this table alone yields no TOTP seeds.
CREATE TABLE user_mfa (
    user_id uuid PRIMARY KEY REFERENCES app_user (id) ON DELETE CASCADE,
    secret_ref text NOT NULL,
    confirmed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- The recovery codes issued at enrolment: each stored only as a hash, the same
-- posture as a session token, so the table holds nothing that could be
-- presented as a code. A code is single-use — used_at stamps the moment it was
-- spent, and a spent code never verifies again.
CREATE TABLE mfa_recovery_code (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id uuid NOT NULL REFERENCES app_user (id) ON DELETE CASCADE,
    code_hash text NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT mfa_recovery_code_hash_key UNIQUE (code_hash)
);

-- Every lookup is "this member's live codes", so the index leads with user_id.
CREATE INDEX idx_mfa_recovery_code_user ON mfa_recovery_code (user_id);
