-- The widened bulk_operation check added beside this file is validated here, in
-- its own transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '5s';
ALTER TABLE bulk_operation VALIDATE CONSTRAINT bulk_operation_verb_check;
