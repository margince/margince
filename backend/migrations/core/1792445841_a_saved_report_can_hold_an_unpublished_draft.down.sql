-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DELETE FROM report_definition_revision r
 USING report_definition d
 WHERE r.report_id = d.id AND r.revision = d.draft_revision;
ALTER TABLE report_definition DROP CONSTRAINT report_definition_draft_after_published;
ALTER TABLE report_definition DROP COLUMN draft_revision;
