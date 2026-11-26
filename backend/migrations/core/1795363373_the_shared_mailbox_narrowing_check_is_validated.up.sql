-- The narrowing reason check widened beside this file is validated here, in
-- its own transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '5s';
ALTER TABLE contact VALIDATE CONSTRAINT contact_narrowing_reason_check;
