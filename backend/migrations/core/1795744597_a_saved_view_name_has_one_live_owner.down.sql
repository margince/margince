-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- pgmigrate:no-transaction
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS idx_saved_view_owner;
CREATE INDEX CONCURRENTLY idx_saved_view_owner ON saved_view (owner_id, resource) WHERE archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS saved_view_name_once;
RESET lock_timeout;
