-- 1789380622: mfa_totp_enrolment_and_recovery_codes.
--
-- The second factor for the password path: a per-user TOTP enrolment, the
-- one-time recovery codes that get a member back in when their authenticator is
-- gone, and the spent-challenge ledger that keeps a login challenge single-use.
-- The enrolment hangs off app_user and cascades with it — an account that is
-- deleted has no second factor to keep — and the recovery codes hang off the
-- ENROLMENT, so disabling the factor removes them the same way deleting the
-- account does.

-- One TOTP enrolment per member. The row exists from the moment enrolment
-- STARTS (the secret is generated and sealed) and confirmed_at stays NULL until
-- the member proves possession by entering a first code — an unconfirmed row is
-- a pending enrolment, not an active factor, so a login challenge must ignore
-- it. The secret itself is NEVER stored here: secret_ref is the keyvault handle
-- to the sealed material, so a dump of this table alone yields no TOTP seeds.
--
-- last_used_step is the highest 30-second TOTP step a code was ever accepted
-- at: a verifier refuses any step at or below it, so an intercepted code cannot
-- be replayed inside the ±1-step skew window. Zero means no code has been
-- accepted under the current secret; restarting an enrolment resets it because
-- a fresh secret opens a fresh step namespace.
CREATE TABLE user_mfa (
    user_id uuid PRIMARY KEY REFERENCES app_user (id) ON DELETE CASCADE,
    secret_ref text NOT NULL,
    confirmed_at timestamptz,
    last_used_step bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- The recovery codes issued at enrolment: each stored only as a hash, the same
-- posture as a session token, so the table holds nothing that could be
-- presented as a code. A code is single-use — used_at stamps the moment it was
-- spent, and a spent code never verifies again.
--
-- The codes reference the ENROLMENT rather than app_user: a recovery code is an
-- alternative to the authenticator, not a credential of its own, so removing
-- the factor (which deletes the user_mfa row) must take every code with it —
-- an app_user reference would strand them live after a disable.
CREATE TABLE mfa_recovery_code (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id uuid NOT NULL REFERENCES user_mfa (user_id) ON DELETE CASCADE,
    code_hash text NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT mfa_recovery_code_hash_key UNIQUE (code_hash)
);

-- Every lookup is "this member's live codes", so the index leads with user_id.
CREATE INDEX idx_mfa_recovery_code_user ON mfa_recovery_code (user_id);

-- Spent second-factor login challenges. The signed challenge a 202 login hands
-- back is stateless, so without a ledger it would verify for its whole TTL as
-- many times as it is presented; each challenge carries a random nonce (jti)
-- and completing it inserts the nonce here, in the same transaction that mints
-- the session — a duplicate insert is a replay and refuses neutrally. Rows are
-- reaped opportunistically once expires_at passes: past that instant the
-- signature itself no longer verifies, so the ledger has nothing left to hold.
CREATE TABLE mfa_challenge_spent (
    jti text PRIMARY KEY,
    expires_at timestamptz NOT NULL
);
