-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE activity DROP COLUMN IF EXISTS guests_left_out;
ALTER TABLE activity_participant
    DROP COLUMN IF EXISTS past_party_cap,
    DROP COLUMN IF EXISTS response;
