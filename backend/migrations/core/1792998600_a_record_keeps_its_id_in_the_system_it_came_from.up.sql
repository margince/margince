-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Bounded for the reason 1789700000 gave: ALTER TABLE takes ACCESS EXCLUSIVE on
-- tables this migration did not create.
SET LOCAL lock_timeout = '2s';

-- THE ID A RECORD HAS IN THE SYSTEM IT CAME FROM, so a caller holding that id
-- can find the record without scanning for it.
--
-- activity and lead already pair `source_system` with `source_id` under a
-- partial unique index. contact, company, deal and project gained only the
-- system (1789700000), which names where a record came from but not which
-- record it was there. This gives them the same pair and the same index.
--
-- It is not a separate table linking a record to its outside ids. Such a table
-- would be a third answer to "where did this come from" beside source_system
-- and import_record_map, and a record could hold several ids per system, which
-- no caller needs. One id per record per system is what the pair says.
--
-- Both columns stay nullable with no backfill: a record created here has no
-- outside id, and a default would invent one.
--
-- The CHECKs are NOT VALID, and the next migration validates them. 128 is the
-- cap capture_trace.source_system already carries.
--
-- The unique indexes are plain CREATE INDEX: this runner wraps a migration in
-- one transaction, which rules out CONCURRENTLY.

ALTER TABLE contact
    ADD COLUMN source_id text,
    ADD CONSTRAINT contact_source_id_needs_a_source
        CHECK (source_id IS NULL OR source_system IS NOT NULL) NOT VALID,
    ADD CONSTRAINT contact_source_id_length
        CHECK (source_id IS NULL OR char_length(source_id) BETWEEN 1 AND 128) NOT VALID;

ALTER TABLE company
    ADD COLUMN source_id text,
    ADD CONSTRAINT company_source_id_needs_a_source
        CHECK (source_id IS NULL OR source_system IS NOT NULL) NOT VALID,
    ADD CONSTRAINT company_source_id_length
        CHECK (source_id IS NULL OR char_length(source_id) BETWEEN 1 AND 128) NOT VALID;

ALTER TABLE deal
    ADD COLUMN source_id text,
    ADD CONSTRAINT deal_source_id_needs_a_source
        CHECK (source_id IS NULL OR source_system IS NOT NULL) NOT VALID,
    ADD CONSTRAINT deal_source_id_length
        CHECK (source_id IS NULL OR char_length(source_id) BETWEEN 1 AND 128) NOT VALID;

ALTER TABLE project
    ADD COLUMN source_id text,
    ADD CONSTRAINT project_source_id_needs_a_source
        CHECK (source_id IS NULL OR source_system IS NOT NULL) NOT VALID,
    ADD CONSTRAINT project_source_id_length
        CHECK (source_id IS NULL OR char_length(source_id) BETWEEN 1 AND 128) NOT VALID;

CREATE UNIQUE INDEX uq_contact_source ON contact (source_system, source_id)
    WHERE source_system IS NOT NULL AND source_id IS NOT NULL;
CREATE UNIQUE INDEX uq_company_source ON company (source_system, source_id)
    WHERE source_system IS NOT NULL AND source_id IS NOT NULL;
CREATE UNIQUE INDEX uq_deal_source ON deal (source_system, source_id)
    WHERE source_system IS NOT NULL AND source_id IS NOT NULL;
CREATE UNIQUE INDEX uq_project_source ON project (source_system, source_id)
    WHERE source_system IS NOT NULL AND source_id IS NOT NULL;

COMMENT ON COLUMN contact.source_id IS
    'The id this record has in source_system. Null on a record created here. Unique with source_system.';
COMMENT ON COLUMN company.source_id IS
    'The id this record has in source_system. Null on a record created here. Unique with source_system.';
COMMENT ON COLUMN deal.source_id IS
    'The id this record has in source_system. Null on a record created here. Unique with source_system.';
COMMENT ON COLUMN project.source_id IS
    'The id this record has in source_system. Null on a record created here. Unique with source_system.';
