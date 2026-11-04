-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Invitations no longer say why they wait.
SET LOCAL lock_timeout = '3s';

DROP TRIGGER IF EXISTS meeting_invitation_attention_follows_status ON meeting_invitation;
DROP FUNCTION IF EXISTS meeting_invitation_attention_follows_status();
DROP TABLE IF EXISTS meeting_calendar_episode;

ALTER TABLE meeting_invitation
    DROP CONSTRAINT IF EXISTS meeting_invitation_attention_reason_check,
    DROP COLUMN IF EXISTS attention_episode,
    DROP COLUMN IF EXISTS attention_reason;
