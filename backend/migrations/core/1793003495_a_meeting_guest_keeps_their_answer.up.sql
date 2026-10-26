-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- What each calendar guest answered, which guests of a crowded meeting are
-- named by address alone, and how many such guests a meeting leaves out.
--
-- Plain columns with constant defaults, added from the catalog alone. The checks
-- are NOT VALID and validated by the next migration in its own transaction.
SET LOCAL lock_timeout = '3s';

-- NULL is a row no calendar stated an answer for: mail, a member, an import.
ALTER TABLE activity_participant
    ADD COLUMN response text,
    ADD CONSTRAINT activity_participant_response_check
        CHECK (response IN ('accepted', 'declined', 'tentative', 'needs_action')) NOT VALID,
    -- A guest past the party cap: shown on the meeting, never a contact or a seat.
    ADD COLUMN past_party_cap boolean DEFAULT false NOT NULL,
    ADD CONSTRAINT activity_participant_past_party_cap_unresolved
        CHECK (NOT past_party_cap OR (user_id IS NULL AND contact_id IS NULL)) NOT VALID;

ALTER TABLE activity
    ADD COLUMN guests_left_out integer DEFAULT 0 NOT NULL,
    ADD CONSTRAINT activity_guests_left_out_check CHECK (guests_left_out >= 0) NOT VALID;
