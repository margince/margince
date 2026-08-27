-- Bounded for the reason the up direction is: these indexes sit on a live table
-- this file did not create, and a DROP that waits forever queues every writer
-- behind it.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_rel_history_project;
DROP INDEX IF EXISTS idx_rel_history_deal;
DROP INDEX IF EXISTS idx_rel_history_counterparty;
DROP INDEX IF EXISTS idx_rel_history_organization;
DROP INDEX IF EXISTS idx_rel_history_person;
