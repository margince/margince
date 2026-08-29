-- A delivery report names ONE failed recipient, but the bounce stamp landed
-- on the whole row: a send with a CC would mark every address on it as the
-- one that refused. The row now records the address the report actually
-- named, so anything derived from bounces (a dead-address marker) can stay
-- per-address instead of blaming the bystanders on the same send.
-- The ALTER takes an ACCESS EXCLUSIVE lock on a live table; bound the wait so
-- a long-running reader makes the migration fail fast and retry, rather than
-- queueing every writer behind it.
SET LOCAL lock_timeout = '3s';
ALTER TABLE comms_outbound
  ADD COLUMN bounce_recipient text;
ALTER TABLE comms_outbound
  ADD CONSTRAINT comms_outbound_bounce_recipient_stated
  CHECK (bounce_recipient IS NULL OR bounced_at IS NOT NULL) NOT VALID;
