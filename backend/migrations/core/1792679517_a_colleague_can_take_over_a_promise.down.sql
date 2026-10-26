-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- The task a colleague took stays, linked to its claim; only the record of who
-- took it and when goes. The audit log still holds both.
ALTER TABLE conversation_claim DROP CONSTRAINT IF EXISTS conversation_claim_taker_fkey;
ALTER TABLE conversation_claim
    DROP COLUMN IF EXISTS taken_at,
    DROP COLUMN IF EXISTS taken_by_user_id;
