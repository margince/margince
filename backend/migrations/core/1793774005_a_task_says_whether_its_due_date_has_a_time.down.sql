-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Tasks no longer say whether their due date has a time.
SET LOCAL lock_timeout = '3s';

DROP TRIGGER IF EXISTS activity_due_time_follows_due_at ON activity;
DROP FUNCTION IF EXISTS activity_due_time_follows_due_at();
ALTER TABLE activity DROP COLUMN IF EXISTS due_has_time;
