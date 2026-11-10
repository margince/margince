-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS comms_outbound_provider_message;
