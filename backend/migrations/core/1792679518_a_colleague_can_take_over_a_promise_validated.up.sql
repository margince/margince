-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- The taker's foreign key was added NOT VALID one migration earlier, on a column
-- every row held as NULL, so this succeeds.
ALTER TABLE conversation_claim VALIDATE CONSTRAINT conversation_claim_taker_fkey;
