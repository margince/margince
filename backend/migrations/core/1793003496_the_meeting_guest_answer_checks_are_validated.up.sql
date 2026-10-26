-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- None can fail: each column is new and every row carries its default.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_participant VALIDATE CONSTRAINT activity_participant_response_check;
ALTER TABLE activity_participant VALIDATE CONSTRAINT activity_participant_past_party_cap_unresolved;
ALTER TABLE activity VALIDATE CONSTRAINT activity_guests_left_out_check;
