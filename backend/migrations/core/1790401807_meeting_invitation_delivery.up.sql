SET LOCAL lock_timeout = '3s';
ALTER TABLE booking_page ADD COLUMN scheduling_policy jsonb;
CREATE TABLE meeting_invitation (
 activity_id uuid PRIMARY KEY REFERENCES activity(id) ON DELETE CASCADE,
 host_user_id uuid NOT NULL REFERENCES app_user(id),
 provider text NOT NULL CHECK (provider IN ('gcal','graphcal')),
 calendar_id text NOT NULL,
 appointment jsonb NOT NULL,
 receipt jsonb,
 status text NOT NULL CHECK (status IN ('pending','confirmed','rescheduling','canceling','needs_attention','canceled','erased')),
 version bigint NOT NULL DEFAULT 1,
 command text NOT NULL DEFAULT 'create' CHECK (command IN ('create','update','cancel')),
 public_intent jsonb,
 cleanup_complete boolean NOT NULL DEFAULT false,
 reminder_status text NOT NULL DEFAULT 'off' CHECK (reminder_status IN ('off','pending','queued','unavailable')),
 management_hash text NOT NULL UNIQUE,
 management_ref text NOT NULL,
 attempts integer NOT NULL DEFAULT 0,
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 lease_until timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX meeting_invitation_due ON meeting_invitation(next_attempt_at) WHERE status IN ('pending', 'rescheduling', 'canceling');
GRANT SELECT, INSERT, UPDATE, DELETE ON meeting_invitation TO margince_app;

CREATE UNIQUE INDEX meeting_invitation_public_intent ON meeting_invitation ((public_intent->>'key_hash')) WHERE public_intent IS NOT NULL;
