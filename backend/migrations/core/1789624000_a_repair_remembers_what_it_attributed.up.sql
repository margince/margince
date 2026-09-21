-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

-- WHAT THE AUTHOR REPAIR HAS ALREADY DONE, so running it twice is safe and
-- running it again with a correction is not refused as a duplicate.
--
-- The repair reads a source system and writes `source_author_id` /
-- `source_author_name` onto rows that already exist here. It runs in batches
-- over tens of thousands of records, from a script, across a network — so it
-- WILL be interrupted, and it will be re-run. Both of those have to be cheap
-- and neither may be destructive.
--
-- Keyed by the record, not by the run. A run is an occasion; the question this
-- table answers is about the ROW ("has this activity been attributed, and from
-- what") and a reader asking it should not have to know which occasion touched
-- it last. That is also why re-running is an UPSERT here rather than an append:
-- the ledger holds the CURRENT answer per record, and its history lives in
-- audit_log with everything else.
--
-- REVISION, not a hash. A content hash alone answers "is this payload the one I
-- already applied", which is the wrong question when a correction is in flight:
-- apply payload A, correct it with B, and a delayed retry of A has a hash that
-- differs from B's, so a hash-only rule would let the stale retry overwrite the
-- correction. The caller stamps a revision that only increases, and a write
-- whose revision is not greater than the stored one is answered as already-done
-- and changes nothing.
--
-- `payload_hash` DECIDES NOTHING. It records what was applied, for an operator
-- reading this table later. It was briefly the "did anything actually change"
-- test, and that cost two rounds of review: the comparison needs the ACTIVITY's
-- row lock and its visibility check, and the ledger has neither, so it belongs
-- against the activity's own columns and now lives there.
CREATE TABLE source_attribution_repair (
    object_type text NOT NULL,
    object_id uuid NOT NULL,
    -- What was decided, kept beside the decision rather than only on the row:
    -- the row can be erased (Art. 17 clears source_author_name) while the
    -- ledger must still be able to say the repair reached it, or the next run
    -- would read the cleared row as unattributed and write the name back.
    source_author_id uuid,
    source_author_name text,
    payload_hash text NOT NULL,
    source_revision bigint NOT NULL,
    -- Free text naming the batch, for an operator reading this table months
    -- later. Not a foreign key: the repair is not an import_run — that table
    -- narrowed to the CSV importer when the HubSpot overlay was retired — and
    -- inventing a run row per batch would put a second, emptier lifecycle
    -- beside the one audit_log already keeps.
    batch_ref text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT source_attribution_repair_pkey PRIMARY KEY (object_type, object_id),
    -- The same set the author columns exist on — ALL SIX, though the route
    -- that writes this table admits only 'activity' today. The record tables
    -- carry the identical column pair and will be repaired the same way, each
    -- behind its own module's write conventions; when they are, they need no
    -- migration, only a wider enum on the wire. A CHECK narrowed to today's
    -- one caller would have to be widened by an ALTER on a table that by then
    -- holds rows, to admit values it was always going to admit.
    --
    -- It is still a closed set rather than free text: a typo'd object_type
    -- would otherwise sit in the ledger claiming a repair nobody can find the
    -- row for.
    CONSTRAINT source_attribution_repair_object_type CHECK (
        object_type IN ('activity', 'contact', 'company', 'deal', 'lead', 'project')),
    -- A revision counts up from one. Zero or negative would make "not greater
    -- than the stored one" unfalsifiable for the first write.
    CONSTRAINT source_attribution_repair_revision CHECK (source_revision > 0));

COMMENT ON TABLE source_attribution_repair IS
    'What the source-author repair has applied per record, so a re-run is a no-op and a correction is not mistaken for a replay.';
