-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- pgmigrate:no-transaction
-- A source name is held once per seat, so a rep's own import never blocks or
-- reveals an admin's run. Workspace imports (own_seat NULL) share one holder.
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS import_run_one_active_per_source_name;
CREATE UNIQUE INDEX CONCURRENTLY import_run_one_active_per_source_name ON import_run (source_name, own_seat) NULLS NOT DISTINCT WHERE connector = 'csv' AND source_name <> '' AND status IN ('validating', 'awaiting_approval', 'running', 'undoing');
RESET lock_timeout;
