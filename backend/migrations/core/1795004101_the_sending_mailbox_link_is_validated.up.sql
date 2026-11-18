SET LOCAL lock_timeout = '5s';

-- Every row staged before the column holds NULL, so each passes.
ALTER TABLE comms_outbound VALIDATE CONSTRAINT comms_outbound_connection_id_fkey;
