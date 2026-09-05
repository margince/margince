-- A company has two marks, because the chrome shows it at two widths.
--
-- The sidebar expanded has room for the lockup a company prints on its
-- letterhead; collapsed it is a 56px rail with a 32px square to put a face in.
-- One picture cannot serve both: a wordmark scaled into that square is a row of
-- illegible strokes, and a badge stretched across the expanded head is a logo
-- nobody chose. So the record carries each, in its own pair of columns, with
-- the same shape the wordmark's pair already has — the bucket path the bytes
-- live at, and what named the mark (the page a read resolved it from, or the
-- file a person chose).
--
-- Only the installation's own company wears a badge today: it is the one record
-- the chrome draws, and no website read resolves one. The columns sit on
-- `organization` beside the wordmark's rather than on a table of their own
-- because the anchor IS an organization row, and splitting one record's two
-- marks across two tables would put the human-precedence rule that governs both
-- on either side of a join.
-- Adding a nullable column takes an ACCESS EXCLUSIVE lock on `organization`
-- for the instant it rewrites the catalog entry. The bound is on ACQUIRING it:
-- an open transaction holding a conflicting lock would otherwise stall every
-- write to the busiest table in the product for as long as this is willing to
-- queue, which is forever.
SET LOCAL lock_timeout = '3s';

ALTER TABLE organization
    ADD COLUMN logo_icon_object_key text,
    ADD COLUMN logo_icon_origin text;
