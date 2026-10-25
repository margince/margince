-- pgmigrate:no-transaction
-- The tombstone of the erasure a privacy request ran is found by its reason
-- "dsr:<request-id>": a replayed fulfilment keeps that tombstone and answers
-- its account, and the proof backfill reads it. Partial on the contact erase
-- rows, which are a small fraction of audit_log.
--
-- Built concurrently, because audit_log takes every audited write and a plain
-- build would block them all until it finished. The drop ahead of the build
-- clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_audit_erasure_reason;
CREATE INDEX CONCURRENTLY idx_audit_erasure_reason ON audit_log ((evidence->>'reason')) WHERE action = 'erase' AND entity_type = 'contact';
