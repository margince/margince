-- pgmigrate:no-transaction
-- The audit log's Actor filter matches a person on actor_id or on_behalf_of,
-- so the rows an agent wrote for them need an index of their own beside
-- idx_audit_actor. Partial, because a person's own rows carry no on_behalf_of.
--
-- Built concurrently, so audit writes are never blocked for the build. The drop
-- ahead of the build clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_audit_on_behalf_of;
CREATE INDEX CONCURRENTLY idx_audit_on_behalf_of ON audit_log (on_behalf_of, occurred_at DESC) WHERE on_behalf_of IS NOT NULL;
