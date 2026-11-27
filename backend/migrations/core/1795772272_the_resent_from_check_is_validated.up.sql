-- Validate the key that names the bounced delivery a send repeats. Its own
-- file, apart from the migration that added it, so the scan runs under SHARE
-- UPDATE EXCLUSIVE rather than under the ACCESS EXCLUSIVE that one held until
-- it committed.
SET LOCAL lock_timeout = '3s';

ALTER TABLE comms_outbound VALIDATE CONSTRAINT comms_outbound_resent_from_fkey;
