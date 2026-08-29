-- A delivery report names ONE failed recipient, but the bounce stamp landed
-- on the whole row: a send with a CC would mark every address on it as the
-- one that refused. The row now records the address the report actually
-- named, so anything derived from bounces (a dead-address marker) can stay
-- per-address instead of blaming the bystanders on the same send.
ALTER TABLE comms_outbound
  ADD COLUMN bounce_recipient text
  CHECK (bounce_recipient IS NULL OR bounced_at IS NOT NULL);
