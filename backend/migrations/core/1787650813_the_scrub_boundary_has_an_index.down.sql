-- Bounded for the same reason the build is: dropping the index takes a lock that
-- blocks writers on audit_log.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_audit_scrub_boundary;
