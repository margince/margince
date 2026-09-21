-- Dropping the table releases foreign keys on deal, close_date_run and
-- audit_log — tables this migration did not create, so the wait is bounded
-- rather than left to block every writer indefinitely.
SET LOCAL lock_timeout = '5s';

DROP TABLE IF EXISTS deal_correction;
