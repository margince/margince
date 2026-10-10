-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The automations list reads each rule's latest counted run and its recent
-- run count on every page load. A run names its rule only as the key's
-- trailing automation id, so the index is over that expression.
SET LOCAL lock_timeout = '3s';
CREATE INDEX workflow_run_by_automation
  ON workflow_run (handler, right(idempotency_key, 36), created_at DESC, id DESC)
  WHERE status <> 'skipped';
