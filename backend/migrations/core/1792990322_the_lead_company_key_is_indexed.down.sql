-- pgmigrate:no-transaction
-- Dropped concurrently, safe to run again.
DROP INDEX CONCURRENTLY IF EXISTS idx_lead_company;
