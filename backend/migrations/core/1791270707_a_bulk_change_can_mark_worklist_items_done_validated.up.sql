-- The widened bulk_operation checks added beside this file are validated here, in
-- their own transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '5s';
ALTER TABLE bulk_operation VALIDATE CONSTRAINT bulk_operation_verb_check;
ALTER TABLE bulk_operation VALIDATE CONSTRAINT bulk_operation_record_type_check;
