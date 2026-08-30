CREATE TABLE federated_identity (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    provider text NOT NULL,
    subject text NOT NULL,
    email_at_link text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT federated_identity_provider_subject_key UNIQUE (provider, subject),
    CONSTRAINT federated_identity_user_provider_key UNIQUE (user_id, provider)
);

-- No standalone index on user_id: the UNIQUE (user_id, provider) constraint
-- above already carries a btree index leading with user_id, which serves
-- every per-user lookup this module makes (resolveFederatedUser,
-- linkFederatedIdentity) without a second index paying write cost on every
-- insert.
