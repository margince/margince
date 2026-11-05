-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Postgres cannot un-validate, so the checks are re-added NOT VALID, as the
-- migration before this one left them.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_participant
    DROP CONSTRAINT activity_participant_response_check,
    DROP CONSTRAINT activity_participant_past_party_cap_unresolved,
    ADD CONSTRAINT activity_participant_response_check
        CHECK (response IN ('accepted', 'declined', 'tentative', 'needs_action')) NOT VALID,
    ADD CONSTRAINT activity_participant_past_party_cap_unresolved
        CHECK (NOT past_party_cap OR (user_id IS NULL AND contact_id IS NULL)) NOT VALID;
ALTER TABLE activity
    DROP CONSTRAINT activity_guests_left_out_check,
    ADD CONSTRAINT activity_guests_left_out_check CHECK (guests_left_out >= 0) NOT VALID;
