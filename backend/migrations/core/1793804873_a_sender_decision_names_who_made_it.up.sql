-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- NULL means a user made the decision: only a user could before this column.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_sender_override ADD COLUMN captured_by text;
