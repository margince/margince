-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Bounded, because ALTER TABLE takes ACCESS EXCLUSIVE on six tables this
-- migration did not create: a writer holding a row lock would otherwise queue
-- every reader behind these statements for as long as it cared to wait.
SET LOCAL lock_timeout = '2s';

-- WHO WROTE IT WHERE IT CAME FROM, which is a different question from who
-- recorded it here.
--
-- `captured_by` answers the second one and must keep answering only that: it is
-- stamped from the authenticated principal, never from a request body, because
-- a caller who could set it could forge the provenance signal every trust
-- decision reads. That rule is load-bearing and this migration does not touch
-- it.
--
-- But it leaves a true thing unsayable. A record imported from another system
-- was typed by somebody, months or years ago, in that system — and under the
-- rule above every imported row says instead that it was written by whoever ran
-- the import. One colleague then appears to have written a decade of somebody
-- else's correspondence: the timeline says "typed by you" over a note a
-- departed colleague wrote, the graph credits the importer with relationships
-- they have never had, and retrieval quotes it all back as their own testimony.
--
-- So the source author goes in its own column, beside the recorder rather than
-- over it, and both facts stay true at once. The precedent is
-- project_health_assessment.source_author, which spells the same distinction
-- for the same reason: who is on record as having judged it, when that is not
-- whoever typed it in.
--
-- TWO COLUMNS, not one. An author who holds a seat here is named by id, so the
-- name follows them when they change it and stays resolvable after they leave —
-- the read that surfaces it must join app_user WITHOUT a liveness filter, the
-- way readEmailParties already does, because who wrote something in August is a
-- fact about August. No read does that yet; this migration only makes the fact
-- storable, and the projection that shows it is a later change.
--
-- An author who never had a seat is named by the only thing the source knew:
-- their name as text. Collapsing the two into a single free-text column would
-- throw away the link for the authors we can actually identify; collapsing them
-- into an id alone would silently drop every author who never worked here.
ALTER TABLE activity
    ADD COLUMN source_author_id uuid REFERENCES app_user(id) ON DELETE SET NULL,
    ADD COLUMN source_author_name text,
    -- An author-other-than-the-recorder is only meaningful for a row that came
    -- from somewhere. A hand-typed row's author IS its recorder, and a claim
    -- otherwise on such a row would be unfalsifiable.
    --
    -- NOT VALID, and that is the whole difference between an additive migration
    -- and an outage. A validated CHECK scans every activity row before it
    -- commits, and this runner executes a migration's entire SQL inside ONE
    -- transaction (dbmigrate.go's inTx) — so the ACCESS EXCLUSIVE lock taken by
    -- the ALTER is held for the length of that scan, on the largest table in
    -- the installation. `lock_timeout` bounds how long we WAIT for a lock, never
    -- how long we hold one.
    --
    -- Nothing is given up by deferring it. NOT VALID enforces the constraint on
    -- every INSERT and UPDATE from this moment on; it declines only to re-prove
    -- the rows already there, and every one of those predates the columns and
    -- therefore has both of them NULL. A later migration may VALIDATE it under a
    -- weaker lock once somebody wants the catalog to say so.
    ADD CONSTRAINT activity_source_author_needs_a_source
        CHECK (source_author_id IS NULL AND source_author_name IS NULL
               OR source_system IS NOT NULL) NOT VALID;

ALTER TABLE contact
    ADD COLUMN source_author_id uuid REFERENCES app_user(id) ON DELETE SET NULL,
    ADD COLUMN source_author_name text;

ALTER TABLE company
    ADD COLUMN source_author_id uuid REFERENCES app_user(id) ON DELETE SET NULL,
    ADD COLUMN source_author_name text;

ALTER TABLE deal
    ADD COLUMN source_author_id uuid REFERENCES app_user(id) ON DELETE SET NULL,
    ADD COLUMN source_author_name text;

ALTER TABLE lead
    ADD COLUMN source_author_id uuid REFERENCES app_user(id) ON DELETE SET NULL,
    ADD COLUMN source_author_name text;

ALTER TABLE project
    ADD COLUMN source_author_id uuid REFERENCES app_user(id) ON DELETE SET NULL,
    ADD COLUMN source_author_name text;

-- ON DELETE SET NULL rather than RESTRICT: nothing in this tree deletes an
-- app_user — a seat is deactivated, which leaves the row — so this arm fires
-- only for a hard delete somebody performs deliberately, and a dangling id
-- would be worse than a null the reader can see. The name column survives it,
-- which is the point of carrying both.

-- NO INDEX HERE, deliberately. An index on source_author_id would serve the
-- repair's own "which rows have I reached" lookup — but nothing reads it yet,
-- and building it costs what the CHECK above was just written to avoid: this
-- runner wraps a migration in one transaction, CREATE INDEX CONCURRENTLY cannot
-- run inside one, and an ordinary build would hold the ALTER's ACCESS EXCLUSIVE
-- locks on all six tables until it finished scanning the largest one.
--
-- It belongs with the writer that needs it, as its own migration, where it can
-- be built CONCURRENTLY outside a transaction against a table whose rows are
-- already there.

COMMENT ON COLUMN activity.source_author_id IS
    'Who wrote this where it came from, when that is a seat here. captured_by stays who recorded it in this installation.';
COMMENT ON COLUMN activity.source_author_name IS
    'Who wrote this where it came from, when they hold no seat here. Free text from the source system.';
