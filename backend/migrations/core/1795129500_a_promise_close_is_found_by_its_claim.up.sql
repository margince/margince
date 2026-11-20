-- pgmigrate:no-transaction
-- Who closed a promise is read from the newest audit row that changed the
-- claim's status, found by claim id. Without this index each such read walks
-- the audit rows of the claim's contact, or all of them once a merge has moved
-- the claim to another contact.
--
-- Built concurrently, so audit writes are never blocked for the build. The drop
-- ahead of the build clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_audit_claim_status;
CREATE INDEX CONCURRENTLY idx_audit_claim_status ON audit_log ((after->>'claim_id'), occurred_at DESC, id DESC) WHERE after ? 'claim_status';
