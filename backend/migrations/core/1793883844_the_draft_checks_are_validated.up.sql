-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validate the checks the migration before this one added NOT VALID.
--
-- Its own file because the runner wraps each file in one transaction, and there
-- the scan would sit under the ACCESS EXCLUSIVE that ALTER holds to commit.
-- It cannot find a violating row: the columns were created NULL in that migration.
SET LOCAL lock_timeout = '3s';

-- One statement, so mail_draft is scanned once for both.
ALTER TABLE mail_draft
    VALIDATE CONSTRAINT mail_draft_schedule_pair,
    VALIDATE CONSTRAINT mail_draft_communication_context;
