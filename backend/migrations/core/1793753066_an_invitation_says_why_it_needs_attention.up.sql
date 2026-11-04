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
