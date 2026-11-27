-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A narrowing still pending is dropped with the column; the posture itself stays.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection DROP COLUMN IF EXISTS history_narrowing_held;
