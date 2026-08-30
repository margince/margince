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
-- every WHERE user_id = ... lookup this module makes (linkFederatedIdentity's
-- read-before-insert, and the CASCADE delete path above) without a second
-- index paying write cost on every insert. resolveFederatedUser's own lookup
-- filters by (provider, subject) instead, which is what the OTHER unique
-- constraint's index already serves.
