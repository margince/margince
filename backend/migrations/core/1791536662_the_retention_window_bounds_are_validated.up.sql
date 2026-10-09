-- The validation of the constraint added in the previous migration. Separate
-- because VALIDATE takes a lock the ADD does not, and a deferral that never
-- validates is a constraint that only binds new rows.
SET LOCAL lock_timeout = '3s';

ALTER TABLE retention_policy VALIDATE CONSTRAINT retention_policy_retain_days_check;
