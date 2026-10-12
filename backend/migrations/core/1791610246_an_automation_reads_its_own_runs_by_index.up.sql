-- pgmigrate:no-transaction
-- The automations list reads each rule's latest counted run and its recent run
-- count by the rule id that ends a run's idempotency key. Built concurrently,
-- because every engine firing writes workflow_run and a plain build blocks those
-- writes for the length of a scan. Each statement is safe to run again: the
-- drop ahead of the build clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS workflow_run_by_automation;
CREATE INDEX CONCURRENTLY workflow_run_by_automation ON workflow_run (handler, right(idempotency_key, 36), created_at DESC, id DESC) WHERE status <> 'skipped';
