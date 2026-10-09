-- The widened skip-reason check added beside this file is validated here,
-- under SHARE UPDATE EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE provider_run VALIDATE CONSTRAINT provider_run_skip_reason_check;
