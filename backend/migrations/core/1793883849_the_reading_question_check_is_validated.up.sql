-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validate the check the migration before this one added NOT VALID. Its own
-- file, so the scan runs under SHARE UPDATE EXCLUSIVE. It cannot fail: every
-- row holds the column's default or a value the check admits.
SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment_extraction VALIDATE CONSTRAINT attachment_extraction_question_check;
