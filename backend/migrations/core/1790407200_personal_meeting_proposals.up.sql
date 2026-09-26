SET LOCAL lock_timeout = '3s';
CREATE TABLE meeting_proposal (
 activity_id uuid PRIMARY KEY REFERENCES activity(id) ON DELETE CASCADE,
 host_user_id uuid NOT NULL REFERENCES app_user(id),
 request jsonb NOT NULL,
 link_ref text NOT NULL,
 token_hash text NOT NULL UNIQUE,
 expires_at timestamptz NOT NULL,
 invitation_id uuid REFERENCES meeting_invitation(activity_id) ON DELETE SET NULL,
 used_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT, INSERT, UPDATE, DELETE ON meeting_proposal TO margince_app;
