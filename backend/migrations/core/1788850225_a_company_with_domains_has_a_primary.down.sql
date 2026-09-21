-- Irreversible by design, and empty rather than a guess.
--
-- The up migration elected a primary for organizations that had none. Undoing
-- it would mean clearing is_primary from rows that carry it — and after the
-- migration nothing tells an elected row apart from one a human chose. Clearing
-- all of them would silently un-enrich companies whose primary was never in
-- question, which is a larger break than the one this reverses.
SELECT 1;
