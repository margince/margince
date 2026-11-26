-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- NOT VALID: rows already holding `personal` stay readable, and the constraint
-- refuses new ones.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_sender_override DROP CONSTRAINT IF EXISTS capture_sender_override_decision_check;
ALTER TABLE capture_sender_override ADD CONSTRAINT capture_sender_override_decision_check
    CHECK (decision IN ('business', 'keep_out')) NOT VALID;
