-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

-- lead keeps its column: it had source_system since the baseline and this
-- migration only added the constraint, so dropping the column here would
-- destroy data the up-migration never created.
ALTER TABLE lead
    DROP CONSTRAINT IF EXISTS lead_source_author_needs_a_source;

ALTER TABLE contact
    DROP CONSTRAINT IF EXISTS contact_source_author_needs_a_source,
    DROP COLUMN IF EXISTS source_system;

ALTER TABLE company
    DROP CONSTRAINT IF EXISTS company_source_author_needs_a_source,
    DROP COLUMN IF EXISTS source_system;

ALTER TABLE deal
    DROP CONSTRAINT IF EXISTS deal_source_author_needs_a_source,
    DROP COLUMN IF EXISTS source_system;

ALTER TABLE project
    DROP CONSTRAINT IF EXISTS project_source_author_needs_a_source,
    DROP COLUMN IF EXISTS source_system;
