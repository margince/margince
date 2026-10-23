-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- Without the column every approval email goes out as before.
ALTER TABLE approval DROP CONSTRAINT IF EXISTS approval_staged_in_conversation_of_fkey;
ALTER TABLE approval DROP COLUMN IF EXISTS staged_in_conversation_of;
