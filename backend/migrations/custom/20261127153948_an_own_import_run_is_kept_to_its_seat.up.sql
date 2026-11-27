-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- own_seat is the seat a run is kept to when its importer imports only their
-- own records; NULL is a workspace import. A source name is then held once per
-- seat, so a rep's name never blocks or reveals an admin's run.
SET LOCAL lock_timeout = '3s';

ALTER TABLE import_run ADD COLUMN IF NOT EXISTS own_seat uuid;
DROP INDEX IF EXISTS import_run_one_active_per_source_name;
CREATE UNIQUE INDEX IF NOT EXISTS import_run_one_active_per_source_name ON import_run
  (source_name, coalesce(own_seat, '00000000-0000-0000-0000-000000000000'::uuid))
  WHERE connector = 'csv' AND source_name <> ''
    AND status IN ('validating', 'awaiting_approval', 'running', 'undoing');
