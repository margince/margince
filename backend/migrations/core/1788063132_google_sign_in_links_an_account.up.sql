CREATE TABLE federated_identity (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES app_user(id),
    provider text NOT NULL,
    subject text NOT NULL,
    email_at_link text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT federated_identity_provider_subject_key UNIQUE (provider, subject),
    CONSTRAINT federated_identity_user_provider_key UNIQUE (user_id, provider)
);

CREATE INDEX idx_federated_identity_user ON federated_identity (user_id);
