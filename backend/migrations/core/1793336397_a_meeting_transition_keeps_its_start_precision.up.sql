-- Whether a meeting transition's scheduled_start is an instant or the anchor
-- of a date with no hour (noon UTC of that date, as activity.occurred_precision
-- spells it).
--
-- NULL means unknown: the row was recorded before this column, and a noon
-- start on such a row cannot say whether it was a time or a date. Rows the old
-- noon repair restored to a known instant are enriched by the data task
-- core/1793336397_meeting_history_precision; no other row is guessed.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_meeting_history ADD COLUMN scheduled_precision text;
-- NOT VALID, and no later migration validates it: the existing rows hold a
-- start whose precision nobody recorded. Postgres still checks every row
-- written from now on, so a new transition with a start says how much of it
-- is known. The IS NOT NULL is load-bearing: IN over a NULL is NULL, and a
-- CHECK lets NULL through.
ALTER TABLE activity_meeting_history ADD CONSTRAINT activity_meeting_history_start_precision
  CHECK (CASE WHEN scheduled_start IS NULL THEN scheduled_precision IS NULL
              ELSE scheduled_precision IS NOT NULL AND scheduled_precision IN ('instant', 'day') END) NOT VALID;
