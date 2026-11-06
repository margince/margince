-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_name_trigram_candidates;
