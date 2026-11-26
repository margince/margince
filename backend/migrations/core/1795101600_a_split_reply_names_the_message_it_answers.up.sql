-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A reply sent on a new topic is filed under a conversation of its own. The
-- delivery names the message it answers, so the owed reply still settles and
-- consent asks that message's conversation at transmit.
SET LOCAL lock_timeout = '3s';

ALTER TABLE comms_outbound
    ADD COLUMN IF NOT EXISTS answers_activity_id uuid REFERENCES activity(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS comms_outbound_answers_ix
    ON comms_outbound (answers_activity_id) WHERE answers_activity_id IS NOT NULL;
