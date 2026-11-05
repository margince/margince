-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- The check was added NOT VALID one migration earlier, on a column every row
-- held as 'unassessed', so this succeeds.
ALTER TABLE activity VALIDATE CONSTRAINT activity_body_completeness_check;
