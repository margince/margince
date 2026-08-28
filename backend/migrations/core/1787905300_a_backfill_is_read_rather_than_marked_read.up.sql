-- Where a thread's reading STARTED, so a backfill is read rather than skipped.
--
-- The scan recorded only how far forward it had got: the newest message's
-- instant and the thread's message count. A backfill changes the count, so the
-- thread becomes due again — and the read then looks at the same newest six
-- messages, finds nothing new, and records the new count. The inserted older
-- messages never reach the model, and the thread now looks read.
--
-- That is the bad direction: the state says handled, nothing errored, and there
-- is no signal that content was skipped.
--
-- scanned_from is the OLDEST message a read has covered. With it the two ends
-- of the conversation are both known, so "unread" has an answer at either end:
-- newer than last_activity_at is new mail, older than scanned_from is a
-- backfill, and a thread walks backwards a window at a time until neither
-- remains.
--
-- NULL on every existing row, which reads as "the start is not known yet". The
-- read treats that as the newest window, which is exactly what it did before —
-- so no thread changes behaviour until it is next scanned.
SET LOCAL lock_timeout = '3s';

-- The id travels with the instant, because an instant is not a message
-- boundary. Mail imported in bulk shares an occurred_at routinely, and a cursor
-- that is a timestamp alone either skips the rest of a group or re-reads it
-- forever. (occurred_at, id) is the order the read already walks in, so the
-- pair is the cursor and the comparison is a tuple.
ALTER TABLE signal_thread_scan
    ADD COLUMN scanned_from timestamptz,
    ADD COLUMN scanned_from_id uuid;

ALTER TABLE signal_thread_scan
    ADD CONSTRAINT signal_thread_scan_scanned_from_shape
    CHECK ((scanned_from IS NULL) = (scanned_from_id IS NULL));
