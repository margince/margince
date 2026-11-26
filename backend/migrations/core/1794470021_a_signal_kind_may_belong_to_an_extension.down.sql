-- Drop the check the up added. The old one is still in place beside it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE signal DROP CONSTRAINT signal_kind_check_v3;
