-- A captured message records the language it is written in.
--
-- The column has existed since the baseline and nothing ever wrote it. Its only
-- reader is the generated search_tsv, which picks a text-search dictionary from
-- it; every row carrying NULL is indexed with the `simple` dictionary, which
-- does no stemming at all.
--
-- Two things change here. The CHECK admitted only de and en while the product
-- ships Vietnamese, so a detected Vietnamese message could not be recorded at
-- all. And capture starts writing what it detects, which means new rows index
-- under a real dictionary.
--
-- Deliberately NO backfill. Writing a language rewrites that row's stored
-- search_tsv, so a backfill would re-stem the entire history in one statement
-- and shift existing search results for every reader at once. New rows carry
-- the label; old rows keep the behaviour they have always had, and the drafting
-- ladder falls through to detection for them.
--
-- The CHECK swap validates every existing row and takes a lock on activity
-- while it does. Rows already satisfy it — the new set is a superset of the old
-- one — so the scan finds nothing, but it is a scan and not a metadata-only
-- change.

-- A bounded wait: the ALTER takes a lock that blocks writers on activity, and
-- an open transaction holding a conflicting one would otherwise stall every
-- write to the table for as long as this migration is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity DROP CONSTRAINT activity_language_check;

ALTER TABLE activity ADD CONSTRAINT activity_language_check
    CHECK (language IS NULL OR language IN ('de', 'en', 'vi'));
