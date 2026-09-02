-- The push subscription's own id, kept beside the deadline it expires at.
--
-- Renewing needs the provider's handle on the subscription, and without it the
-- Graph connector went looking: GET /subscriptions, paged, once per mailbox per
-- renewal cycle, matching on the notification URL. That listing is a RECOVERY
-- path — it finds a subscription this installation lost track of — and it was
-- doing steady-state work.
--
-- NULLABLE, and it stays that way. Every connection made before this column
-- existed has no id to backfill, and a mailbox on another provider has no
-- subscription at all. A NULL means "no handle stored", and the renewal falls
-- back to the listing it has always used, which is where a recovery path
-- belongs.
--
-- Bounded, like every migration that locks a table it did not create: an open
-- transaction holding a conflicting lock would otherwise stall every write to
-- capture_connection for as long as this is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection ADD COLUMN watch_ref text;
