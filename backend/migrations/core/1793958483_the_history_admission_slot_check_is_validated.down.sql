-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_backfill
 DROP CONSTRAINT capture_backfill_admission_slot_check,
 ADD CONSTRAINT capture_backfill_admission_slot_check CHECK (admission_slot > 0) NOT VALID;
