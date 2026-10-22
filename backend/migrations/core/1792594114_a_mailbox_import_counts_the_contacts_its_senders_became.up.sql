-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The backfill run that queued a sender for a verdict. The verdict creates the
-- contact later, outside the run's page, and this is what credits the run with
-- it. NULL for a sender live sync queued, which belongs to no run.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_pending_counterparty
    ADD COLUMN backfill_id uuid REFERENCES capture_backfill (id) ON DELETE SET NULL;

CREATE INDEX capture_pending_counterparty_backfill_idx
    ON capture_pending_counterparty (backfill_id) WHERE backfill_id IS NOT NULL;
