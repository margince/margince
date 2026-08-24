-- A site read may now replace an organization description that no person
-- authored, and it decides who authored one by looking for a field_provenance
-- row. Every description written before this migration predates that stamp, so
-- without a backfill the first crawl after deployment would read "no row" as
-- "nobody's words" and overwrite descriptions people typed.
--
-- The row's own captured_by is the best evidence available about who wrote the
-- description on a record whose fields were never stamped individually: it is
-- what storekit.FieldOrigins already falls back to for exactly this case. That
-- makes the backfill say no more than the data supports — a company a person
-- created keeps their claim, one an agent or an import created does not gain a
-- claim it never had.
--
-- Only rows with no description stamp already, so re-running changes nothing
-- and a stamp written by the new code is never overwritten by an older answer.
INSERT INTO field_provenance (object_type, object_id, field_name, source, captured_by, captured_at)
SELECT 'organization', o.id, 'description', o.source, o.captured_by, o.created_at
FROM organization o
WHERE o.description IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM field_provenance fp
    WHERE fp.object_type = 'organization'
      AND fp.object_id = o.id
      AND fp.field_name = 'description');
