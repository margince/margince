-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- pgmigrate:no-transaction
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS import_run_one_active_per_source_name;
CREATE UNIQUE INDEX CONCURRENTLY import_run_one_active_per_source_name ON import_run (source_name) WHERE connector = 'csv' AND source_name <> '' AND status IN ('validating', 'awaiting_approval', 'running', 'undoing');
RESET lock_timeout;
