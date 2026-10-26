-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- The decider constraints were added NOT VALID one migration earlier, so that
-- adding them took no long exclusive lock. Validating them here checks the rows
-- already in the table under a lock readers and writers pass. Every pending row
-- was given a decider or closed by that migration, so these succeed.
ALTER TABLE approval VALIDATE CONSTRAINT approval_requested_decider_fkey;
ALTER TABLE approval VALIDATE CONSTRAINT approval_requested_decider_source_check;
ALTER TABLE approval VALIDATE CONSTRAINT approval_pending_names_decider;
