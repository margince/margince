-- A line asking for a decision stops being answerable the moment somebody else
-- answers it, and the reader it was addressed to has not read anything.
--
-- read_at cannot say this. It records that a human looked, and a badge cleared
-- by machinery that writes "they read it" makes the table lie about the one
-- fact it exists to hold. The row itself has to stay: the unique index on
-- (recipient, dedupe_key) is what makes a redelivered announcement free, so
-- deleting the line would let the next replay of the same envelope put it back.
SET LOCAL lock_timeout = '3s';

ALTER TABLE notice ADD COLUMN retracted_at timestamptz;
