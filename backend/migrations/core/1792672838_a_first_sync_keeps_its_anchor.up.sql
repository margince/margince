SET LOCAL lock_timeout = '3s';

-- The point a first sync reads from, held across retries.
--
-- Derived fresh on each attempt, a retry after a long outage anchors later than
-- the one before it and the mail in between is read by neither. Held here, every
-- retry of a first sync reads the same point.
ALTER TABLE capture_connection
  ADD COLUMN sync_anchor timestamptz;

-- The period no sync read, when an outage outlasted the anchor's own limit.
--
-- Both or neither: a half-recorded gap names a start with no end, which a reader
-- cannot act on and the backfill cannot be pointed at.
ALTER TABLE capture_connection
  ADD COLUMN unread_from timestamptz,
  ADD COLUMN unread_to timestamptz;

ALTER TABLE capture_connection
  ADD CONSTRAINT capture_connection_unread_period_whole CHECK (
    (unread_from IS NULL AND unread_to IS NULL)
    OR (unread_from IS NOT NULL AND unread_to IS NOT NULL AND unread_from < unread_to)
  ) NOT VALID;
