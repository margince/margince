-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS event_outbox_audit_record;
CREATE INDEX CONCURRENTLY event_outbox_audit_record ON event_outbox ((envelope->'trace'->>'audit_log_id'));
DROP INDEX CONCURRENTLY IF EXISTS event_outbox_entity_record;
CREATE INDEX CONCURRENTLY event_outbox_entity_record ON event_outbox ((envelope->'entity'->>'type'), (envelope->'entity'->>'id'));
