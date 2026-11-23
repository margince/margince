-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS deal_correction_note_ix;
ALTER TABLE deal_correction DROP COLUMN IF EXISTS note_activity_id;
