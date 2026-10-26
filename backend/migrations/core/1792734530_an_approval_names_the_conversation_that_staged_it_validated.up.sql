-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- The foreign key was added NOT VALID one migration earlier, on a column every
-- row held as NULL, so this succeeds.
ALTER TABLE approval VALIDATE CONSTRAINT approval_staged_in_conversation_of_fkey;
