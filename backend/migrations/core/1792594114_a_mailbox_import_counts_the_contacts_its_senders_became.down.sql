-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS capture_pending_counterparty_backfill_idx;
ALTER TABLE capture_pending_counterparty DROP COLUMN IF EXISTS backfill_id;
