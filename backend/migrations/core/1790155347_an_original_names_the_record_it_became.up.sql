-- An original names the record it became, so nothing has to reconstruct a key.
--
-- raw_capture.source_id has two writers and two meanings: the mail sink stores
-- the domain natural key, the channel poll stores the provider's redelivery
-- key. A correlation keyed on that pair is therefore true for one lane and
-- silently false for the other -- zero rows, no error, success reported. A
-- reference has one answer for every lane, and a wrong one cannot be silent.
--
-- NULL is ordinary, not a defect: an activity typed by hand or produced by a
-- connector that keeps no original has none to name, and a purge nulls it as it
-- destroys the row it pointed at.
--
-- SET NULL and never CASCADE. The original ages out on its own clock while the
-- correspondence it was read from stays on the record; a cascade would make
-- destroying the evidence destroy the message.

SET LOCAL lock_timeout = '5s';

ALTER TABLE activity
    ADD COLUMN raw_capture_id uuid REFERENCES raw_capture (id) ON DELETE SET NULL;

-- Read by the purge and the age-out selector, and walked by the FK on every
-- raw_capture delete. Partial: the rows that matter are the ones that name an
-- original, and hand-typed activity rows never will.
CREATE INDEX activity_raw_capture_id_idx ON activity (raw_capture_id)
    WHERE raw_capture_id IS NOT NULL;
