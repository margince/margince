SET LOCAL lock_timeout = '3s';

ALTER TABLE retention_policy DROP CONSTRAINT retention_policy_retain_days_check;
