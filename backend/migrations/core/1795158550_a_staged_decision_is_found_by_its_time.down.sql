-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS communication_decision_staged_at;
