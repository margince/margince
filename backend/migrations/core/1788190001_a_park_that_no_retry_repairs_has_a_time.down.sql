SET LOCAL lock_timeout = '3s';

DROP INDEX idx_comms_outbound_parked;
ALTER TABLE comms_outbound DROP COLUMN parked_at;
