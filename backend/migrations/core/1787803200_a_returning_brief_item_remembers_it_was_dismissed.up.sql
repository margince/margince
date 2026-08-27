-- A deal the rep dismissed and that has come back says so.
--
-- The suppression rule (briefrank.go's candidate query) holds a dismissed deal
-- out of every later queue until a linked activity occurs after the mark. So a
-- deal that reappears is ALWAYS a deal the rep waved away and that has since
-- moved — and until now the card said nothing about it. The rep sees the same
-- deal they dismissed on Tuesday sitting at rank 2 on Thursday with no
-- explanation, which reads as the product having forgotten.
--
-- WHY NOT REUSE state_at. brief_item_state_stamped enforces
-- (state = 'new') = (state_at IS NULL), so a returning item — which IS new in
-- its own run — cannot carry the previous mark's timestamp in that column.
-- These are a different fact anyway: state_at is what the rep did to THIS row,
-- and these say what they did to an earlier one.
--
-- WHY ON THE ROW rather than derived at read time. The derivation would be a
-- cross-run query per item on the read path, and the answer never changes once
-- the run is assembled: the dismissal it names has already happened. Stamping
-- it when the run is built is one write instead of a read every morning.
--
-- Bounded, because both statements block writers on a table this migration did
-- not create.
SET LOCAL lock_timeout = '3s';

ALTER TABLE brief_item ADD COLUMN returned_after_dismissal_on date;

-- The local day the rep dismissed this deal on, in the installation's
-- reporting zone — the same zone brief_run.local_day is stamped in, so "you
-- dismissed it Tuesday" and "this is Thursday's brief" are measured the same
-- way. A DATE and not an instant: the sentence names a day, and storing an
-- instant would invite a reader to render a time nobody promised was accurate.

ALTER TABLE brief_item ADD COLUMN returned_with_activity_at timestamptz;

-- When the activity that brought it back occurred. THIS one is an instant,
-- because it is a specific event and the card may say how long ago it was.
--
-- The pair is all-or-nothing: an item either carries both halves of the
-- lineage or neither. Half of it — a dismissal day with no reason it returned,
-- or a reason with no dismissal — is a sentence the screen cannot finish.
ALTER TABLE brief_item
    ADD CONSTRAINT brief_item_lineage_whole
    CHECK ((returned_after_dismissal_on IS NULL) = (returned_with_activity_at IS NULL));
