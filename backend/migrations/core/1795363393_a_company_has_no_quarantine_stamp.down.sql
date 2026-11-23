-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The column comes back empty; it never held a value.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company ADD COLUMN IF NOT EXISTS quarantined_at timestamptz;
