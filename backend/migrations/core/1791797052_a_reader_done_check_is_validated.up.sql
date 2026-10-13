-- The two reader-state checks added beside this file are validated here, under
-- SHARE UPDATE EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_reader_state VALIDATE CONSTRAINT activity_reader_state_known;
ALTER TABLE activity_reader_state VALIDATE CONSTRAINT activity_reader_state_shaped;
