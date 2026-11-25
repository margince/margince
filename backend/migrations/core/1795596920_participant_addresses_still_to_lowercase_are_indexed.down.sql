-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_aparticipant_address_to_lowercase;
