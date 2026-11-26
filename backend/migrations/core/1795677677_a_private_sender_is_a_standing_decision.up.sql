-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A `personal` verdict is a standing decision for the address, kept beside the
-- decisions a person types. The table holds one row per seat and sender, so no
-- new table is needed and the constraint swap scans little.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_sender_override DROP CONSTRAINT IF EXISTS capture_sender_override_decision_check;
ALTER TABLE capture_sender_override ADD CONSTRAINT capture_sender_override_decision_check
    CHECK (decision IN ('business', 'keep_out', 'personal'));
