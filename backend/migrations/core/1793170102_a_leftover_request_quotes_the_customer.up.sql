-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

-- The settlement rule a verdict was reached under. A still_owed verdict from
-- before the leftover quoted the customer's own request carries '' and is
-- judged again.
ALTER TABLE activity_request_settlement ADD COLUMN IF NOT EXISTS rule_version text NOT NULL DEFAULT '';
