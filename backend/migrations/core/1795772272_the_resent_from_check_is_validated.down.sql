SET LOCAL lock_timeout = '3s';

ALTER TABLE comms_outbound
    DROP CONSTRAINT IF EXISTS comms_outbound_resent_from_fkey,
    ADD CONSTRAINT comms_outbound_resent_from_fkey
    FOREIGN KEY (resent_from) REFERENCES comms_outbound (id) ON DELETE SET NULL NOT VALID;
