-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A change Margince made names the History note that reports it, so the note
-- can offer the same Undo the change has. A change made before this column
-- existed has no note link and keeps only the deal's header line.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_correction
    ADD COLUMN IF NOT EXISTS note_activity_id uuid REFERENCES activity(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS deal_correction_note_ix
    ON deal_correction (note_activity_id) WHERE note_activity_id IS NOT NULL;
