-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A mailbox whose posture change asked to cover the mail it already brought in
-- records that request here, and a background job narrows that history batch by
-- batch. NULL is nothing pending; false narrows to classified, true to held.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection ADD COLUMN IF NOT EXISTS history_narrowing_held boolean;
