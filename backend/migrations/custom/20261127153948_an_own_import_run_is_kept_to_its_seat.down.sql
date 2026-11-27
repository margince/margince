-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS import_run_one_active_per_source_name;
CREATE UNIQUE INDEX IF NOT EXISTS import_run_one_active_per_source_name ON import_run (source_name)
  WHERE connector = 'csv' AND source_name <> ''
    AND status IN ('validating', 'awaiting_approval', 'running', 'undoing');
ALTER TABLE import_run DROP COLUMN IF EXISTS own_seat;
