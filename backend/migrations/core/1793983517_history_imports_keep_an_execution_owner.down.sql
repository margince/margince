-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DROP INDEX uq_capture_backfill_admission_slot;
ALTER TABLE capture_backfill
    DROP CONSTRAINT capture_backfill_admission_slot_check,
    DROP COLUMN execution_due_at,
    DROP COLUMN execution_owner_pid,
    DROP COLUMN admission_slot,
    DROP COLUMN execution_generation;
