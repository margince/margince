-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- An editor can save a report's next revision as a draft. Readers keep seeing
-- the published revision until the editor publishes. The draft is always the
-- newest revision, so evaluation, which serves revisions up to the published
-- one, never reaches it. NOT VALID skips the row scan; the column is new.
SET LOCAL lock_timeout = '3s';

ALTER TABLE report_definition ADD COLUMN draft_revision bigint;
ALTER TABLE report_definition ADD CONSTRAINT report_definition_draft_after_published
    CHECK (draft_revision IS NULL OR draft_revision BETWEEN revision + 1 AND 9007199254740991) NOT VALID;
