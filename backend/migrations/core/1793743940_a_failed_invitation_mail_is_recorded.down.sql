-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Invitation mail outcomes are no longer recorded.
SET LOCAL lock_timeout = '3s';

ALTER TABLE auth_token
    DROP COLUMN IF EXISTS mail_failed_at,
    DROP COLUMN IF EXISTS mail_sent_at,
    DROP COLUMN IF EXISTS mail_queued_at;
