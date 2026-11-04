-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Why an invitation stopped and waits for its host. calendar_disconnected parks
-- every queued invitation of a host whose calendar refused us; delivery_failed
-- is one invitation that failed its last attempt. attention_episode groups the
-- rows one disconnect parked, so the host is told once. Rows already waiting
-- keep NULL, read as "reason unknown". The check is validated in the next
-- migration, outside this one's lock.
SET LOCAL lock_timeout = '3s';

ALTER TABLE meeting_invitation
    ADD COLUMN attention_reason text,
    ADD COLUMN attention_episode uuid,
    ADD CONSTRAINT meeting_invitation_attention_reason_check
        CHECK (attention_reason IN ('delivery_failed', 'calendar_disconnected')) NOT VALID;

-- The latest disconnect of each host's calendar. It outlives the invitations
-- it parked, so a rejection that was in flight when they resumed is told apart
-- from a new disconnect, and the host is not told twice about one.
CREATE TABLE meeting_calendar_episode (
    host_user_id uuid NOT NULL,
    provider text NOT NULL,
    episode uuid NOT NULL,
    started_at timestamptz NOT NULL,
    resumed_at timestamptz,

    CONSTRAINT meeting_calendar_episode_pkey PRIMARY KEY (host_user_id, provider),
    CONSTRAINT meeting_calendar_episode_host_user_id_fkey FOREIGN KEY (host_user_id) REFERENCES app_user(id) ON DELETE CASCADE,
    CONSTRAINT meeting_calendar_episode_provider_check CHECK (provider IN ('gcal', 'graphcal'))
);

-- A reason exists only while the invitation waits. Every writer that moves the
-- status on (a retry, a cancel, a delivery, an erasure) leaves it behind here,
-- so no writer has to remember to clear it.
CREATE FUNCTION meeting_invitation_attention_follows_status() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status <> 'needs_attention' THEN
    NEW.attention_reason := NULL;
    NEW.attention_episode := NULL;
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER meeting_invitation_attention_follows_status BEFORE INSERT OR UPDATE ON meeting_invitation
  FOR EACH ROW EXECUTE FUNCTION meeting_invitation_attention_follows_status();
