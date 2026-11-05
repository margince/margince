-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validates the two draft CHECKs the previous migration added NOT VALID, in a
-- transaction of its own so the scan runs under SHARE UPDATE EXCLUSIVE. Every
-- stored row passes: the column was NULL on every row when it was added.
SET LOCAL lock_timeout = '3s';

ALTER TABLE report_definition VALIDATE CONSTRAINT report_definition_draft_after_published;
ALTER TABLE report_definition VALIDATE CONSTRAINT report_definition_draft_revision_js_safe;
