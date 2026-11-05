-- Validation is not revertible: a validated constraint stays validated, and the
-- bounds it checks are dropped by the file beside it. origin carries no default,
-- so this statement changes nothing.
SET LOCAL lock_timeout = '5s';

ALTER TABLE lead ALTER COLUMN origin DROP DEFAULT;
