-- Validate the audit image check.
--
-- An image column holds the record's before or after state, or nothing at all;
-- the JSON literal `null` is neither, and a reader cannot tell it from an
-- absent image. The rule has bound new rows since it was added — audit_log is
-- append-only, so that is every row written since — and was never checked
-- against what came before it.
--
-- Its own file so the scan runs under SHARE UPDATE EXCLUSIVE rather than under
-- the ACCESS EXCLUSIVE the adding migration held until it committed.
SET LOCAL lock_timeout = '3s';

ALTER TABLE audit_log VALIDATE CONSTRAINT audit_log_images_are_absent_or_present;
