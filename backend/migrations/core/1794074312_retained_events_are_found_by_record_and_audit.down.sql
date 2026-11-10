-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS event_outbox_audit_record;
DROP INDEX CONCURRENTLY IF EXISTS event_outbox_entity_record;
