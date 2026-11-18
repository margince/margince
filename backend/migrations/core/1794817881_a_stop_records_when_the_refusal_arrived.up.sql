SET LOCAL lock_timeout = '3s';
-- When the refusal reached the controller: a reply mail's arrival, or the time
-- of the call a colleague relays. recorded_at stays the time sending stopped.
-- Rows written before this column read recorded_at for both.
ALTER TABLE communication_suppression ADD COLUMN received_at timestamptz;
