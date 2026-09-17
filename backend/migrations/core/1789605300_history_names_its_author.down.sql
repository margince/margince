-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

ALTER TABLE activity
    DROP CONSTRAINT IF EXISTS activity_source_author_needs_a_source,
    DROP COLUMN IF EXISTS source_author_name,
    DROP COLUMN IF EXISTS source_author_id;

ALTER TABLE contact
    DROP COLUMN IF EXISTS source_author_name,
    DROP COLUMN IF EXISTS source_author_id;

ALTER TABLE company
    DROP COLUMN IF EXISTS source_author_name,
    DROP COLUMN IF EXISTS source_author_id;

ALTER TABLE deal
    DROP COLUMN IF EXISTS source_author_name,
    DROP COLUMN IF EXISTS source_author_id;

ALTER TABLE lead
    DROP COLUMN IF EXISTS source_author_name,
    DROP COLUMN IF EXISTS source_author_id;

ALTER TABLE project
    DROP COLUMN IF EXISTS source_author_name,
    DROP COLUMN IF EXISTS source_author_id;
