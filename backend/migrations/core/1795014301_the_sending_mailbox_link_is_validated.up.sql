SET LOCAL lock_timeout = '5s';

-- Every row staged before the column holds NULL, and every container rule
-- names its member's own connection, so each passes.
ALTER TABLE comms_outbound VALIDATE CONSTRAINT comms_outbound_connection_id_fkey;
ALTER TABLE capture_exclusion VALIDATE CONSTRAINT capture_exclusion_connection_id_fkey;
