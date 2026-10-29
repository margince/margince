-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- One attendee's own "I'm ready" for one meeting occurrence, at the
-- preparation revision they confirmed. The facts the revision was computed
-- from are kept beside it, so a later read can name which of them changed.
CREATE TABLE meeting_readiness (
  attendee_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
  activity_id uuid NOT NULL REFERENCES activity(id) ON DELETE CASCADE,
  -- The calendar occurrence identity, NULL for a meeting with none.
  occurrence_key text,
  revision text NOT NULL,
  starts_at timestamptz NOT NULL,
  starts_on_day boolean NOT NULL,
  agenda_fingerprint text NOT NULL,
  participants_fingerprint text NOT NULL,
  captured_by text NOT NULL,
  confirmed_at timestamptz NOT NULL,
  PRIMARY KEY (attendee_id, activity_id),
  CONSTRAINT meeting_readiness_revision_present CHECK (revision <> '')
);

-- The activity's own cascade looks rows up by activity.
CREATE INDEX idx_meeting_readiness_activity ON meeting_readiness (activity_id);

COMMENT ON TABLE meeting_readiness IS
    'One attendee''s confirmation that they are ready for one meeting, at the preparation revision they confirmed.';
