-- Validate the check the migration before this one added NOT VALID. Its own
-- file, so the scan runs under SHARE UPDATE EXCLUSIVE. It cannot fail: the
-- rule only got looser.
SET LOCAL lock_timeout = '3s';

ALTER TABLE dedupe_scan_state VALIDATE CONSTRAINT dedupe_scan_state_stream_check;
