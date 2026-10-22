-- Validate the phase check the migration before this one added NOT VALID. Its
-- own file, so the scan runs under SHARE UPDATE EXCLUSIVE and import writes go on.
SET LOCAL lock_timeout = '3s';

ALTER TABLE import_run VALIDATE CONSTRAINT import_run_phase_check;
