SET LOCAL lock_timeout = '3s';

-- A previewed count can be a FLOOR, and the run has to remember which it was.
--
-- Gmail's estimate is counted by paging message ids under a cap, so a large
-- mailbox returns "at least N" rather than N. Graph answers an exact count and
-- never does. The preview can say which it has because it has just asked; the
-- progress bar reads the number back hours later, from this row, and without
-- this column it cannot tell a total from a bound — so it divides by a floor
-- and runs past its own end on exactly the mailboxes where the cap binds.
--
-- NOT NULL DEFAULT false, which states the right thing about existing rows:
-- every run recorded before this column existed was previewed under a cap that
-- nobody asked about, and the number it stored was treated as exact. Calling
-- them exact keeps them reading as they always have, and a run that ends is
-- measured by its own counters anyway.
ALTER TABLE capture_backfill
    ADD COLUMN total_estimate_is_floor boolean NOT NULL DEFAULT false;
