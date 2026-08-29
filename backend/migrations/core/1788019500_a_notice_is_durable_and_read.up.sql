-- A notice is the informational half the Worklist lacked: an automation's
-- notify firing and the lead-SLA's notify half had no transport, so both
-- shipped as honest skips. The row is the transport — durable, per
-- recipient, with its OWN read-state — so recording one IS delivering it,
-- and the engine's "recorded successful once Notify returns" stays true.
SET LOCAL lock_timeout = '3s';
CREATE TABLE notice (
  id uuid PRIMARY KEY DEFAULT uuidv7(),
  recipient_user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
  kind text NOT NULL,
  subject text NOT NULL,
  body text NOT NULL DEFAULT '',
  captured_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  read_at timestamptz,
  CONSTRAINT notice_subject_present CHECK (subject <> ''),
  -- The bounds the one writer truncates to, held by the table so a second
  -- writer cannot silently exceed what every open tab receives.
  CONSTRAINT notice_content_bounded CHECK (length(subject) <= 200 AND length(body) <= 2000)
);
-- The lane's read: one recipient's unread notices, newest first.
CREATE INDEX notice_unread ON notice (recipient_user_id, created_at DESC) WHERE read_at IS NULL;
