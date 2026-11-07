-- Validate the check the migration before this one added NOT VALID. Its own
-- file, so the scan runs under SHARE UPDATE EXCLUSIVE and geocode writes do not
-- queue behind it. It cannot fail: the column it reads was added empty.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company_geocode_state VALIDATE CONSTRAINT company_geocode_state_status_check;
