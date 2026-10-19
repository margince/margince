-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE report_definition DROP CONSTRAINT report_definition_draft_revision_js_safe;
ALTER TABLE report_definition DROP CONSTRAINT report_definition_draft_after_published;
ALTER TABLE report_definition ADD CONSTRAINT report_definition_draft_after_published
    CHECK (draft_revision IS NULL OR draft_revision BETWEEN revision + 1 AND 9007199254740991) NOT VALID;
ALTER TABLE report_definition ADD CONSTRAINT report_definition_draft_revision_js_safe
    CHECK (draft_revision BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
