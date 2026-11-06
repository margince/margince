-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_backfill
    ADD COLUMN execution_generation bigint NOT NULL DEFAULT 0,
    ADD COLUMN admission_slot integer,
    ADD COLUMN execution_owner_pid integer,
    ADD COLUMN execution_due_at timestamptz,
    ADD CONSTRAINT capture_backfill_admission_slot_check CHECK (admission_slot > 0) NOT VALID;

CREATE UNIQUE INDEX uq_capture_backfill_admission_slot ON capture_backfill (admission_slot)
    WHERE status = 'running' AND admission_slot IS NOT NULL;
