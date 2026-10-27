-- Validate the check that admits an extension's kinds, then swap it in for the
-- closed one. VALIDATE takes only SHARE UPDATE EXCLUSIVE, and it runs in a file
-- of its own so no ACCESS EXCLUSIVE from the ADD is held beside it.
--
-- The drop and the rename are one transaction with the validation, so a failure
-- rolls all three back and `signal` is never left without a kind constraint.
SET LOCAL lock_timeout = '3s';

ALTER TABLE signal VALIDATE CONSTRAINT signal_kind_check_v3;

ALTER TABLE signal DROP CONSTRAINT signal_kind_check;

ALTER TABLE signal RENAME CONSTRAINT signal_kind_check_v3 TO signal_kind_check;
