-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Bounded, because ALTER TABLE takes ACCESS EXCLUSIVE on five tables this
-- migration did not create: a writer holding a row lock would otherwise queue
-- every reader behind these statements for as long as it cared to wait.
SET LOCAL lock_timeout = '2s';

-- WHERE A RECORD CAME FROM, so that saying who wrote it can be refused on a
-- record nobody imported.
--
-- 1789605300 gave six tables the two author columns and gave ONE of them —
-- activity — the constraint that makes them meaningful: an author other than
-- the recorder is only a claim you can check on a row that came from somewhere,
-- because a hand-typed row's author IS its recorder and a claim otherwise is
-- unfalsifiable. That constraint reads `source_system IS NOT NULL`.
--
-- activity and lead already carried `source_system`. contact, company, deal and
-- project did not, so the constraint could not be written for them and was not.
-- The repair that fills the author columns is now reaching those four, and
-- without this it would have no way to tell an imported company from one a rep
-- typed last week — it would attribute either.
--
-- So the column lands on the four that lack it, and the CHECK lands on all
-- five. lead is included deliberately: it has had the column since the baseline
-- and the author columns since 1789605300, with nothing tying them together, so
-- it is the one table where an author can today be written onto a row from
-- nowhere.
--
-- NULLABLE, with no default and no backfill. A record that names no source was
-- created here, and that is the truth for almost every row in these tables; a
-- default would assert an origin for all of them. The importer writes it going
-- forward, and the rows the HubSpot import already created are a data repair
-- rather than a schema change.

ALTER TABLE contact
    ADD COLUMN source_system text,
    -- NOT VALID, and that is the whole difference between an additive migration
    -- and an outage. A validated CHECK scans every row before it commits, and
    -- this runner executes a migration's entire SQL inside ONE transaction
    -- (dbmigrate.go's inTx) — so the ACCESS EXCLUSIVE lock taken by the ALTER
    -- is held for the length of that scan. `lock_timeout` bounds how long we
    -- WAIT for a lock, never how long we hold one.
    --
    -- Nothing is given up. NOT VALID enforces the constraint on every INSERT
    -- and UPDATE from this moment on; it declines only to re-prove the rows
    -- already there, and every one of those has both author columns NULL
    -- because nothing has ever written them on these tables.
    ADD CONSTRAINT contact_source_author_needs_a_source
        CHECK (source_author_id IS NULL AND source_author_name IS NULL
               OR source_system IS NOT NULL) NOT VALID;

ALTER TABLE company
    ADD COLUMN source_system text,
    ADD CONSTRAINT company_source_author_needs_a_source
        CHECK (source_author_id IS NULL AND source_author_name IS NULL
               OR source_system IS NOT NULL) NOT VALID;

ALTER TABLE deal
    ADD COLUMN source_system text,
    ADD CONSTRAINT deal_source_author_needs_a_source
        CHECK (source_author_id IS NULL AND source_author_name IS NULL
               OR source_system IS NOT NULL) NOT VALID;

ALTER TABLE project
    ADD COLUMN source_system text,
    ADD CONSTRAINT project_source_author_needs_a_source
        CHECK (source_author_id IS NULL AND source_author_name IS NULL
               OR source_system IS NOT NULL) NOT VALID;

-- lead keeps the column it already had and gains only the constraint.
ALTER TABLE lead
    ADD CONSTRAINT lead_source_author_needs_a_source
        CHECK (source_author_id IS NULL AND source_author_name IS NULL
               OR source_system IS NOT NULL) NOT VALID;

COMMENT ON COLUMN contact.source_system IS
    'Which system this record was imported from. Null on a record created here, which is most of them. It is what makes an author other than the recorder a checkable claim rather than an assertion.';
COMMENT ON COLUMN company.source_system IS
    'Which system this record was imported from. Null on a record created here, which is most of them. It is what makes an author other than the recorder a checkable claim rather than an assertion.';
COMMENT ON COLUMN deal.source_system IS
    'Which system this record was imported from. Null on a record created here, which is most of them. It is what makes an author other than the recorder a checkable claim rather than an assertion.';
COMMENT ON COLUMN project.source_system IS
    'Which system this record was imported from. Null on a record created here, which is most of them. It is what makes an author other than the recorder a checkable claim rather than an assertion.';

-- NO INDEX, for the reason 1789605300 gave for the same decision: nothing reads
-- these columns as a predicate yet, this runner wraps a migration in one
-- transaction so CREATE INDEX CONCURRENTLY cannot run here, and an ordinary
-- build would hold the ALTERs' ACCESS EXCLUSIVE locks on all five tables until
-- it finished scanning the largest. It belongs with the reader that needs it.
