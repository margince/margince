-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS conversation_claim_task_ix;
