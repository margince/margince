SET LOCAL lock_timeout = '3s';
ALTER TABLE comms_outbound DROP COLUMN bounce_recipient;
