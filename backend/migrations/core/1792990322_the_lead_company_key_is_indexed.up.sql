-- pgmigrate:no-transaction
-- The index a company delete finds its linked leads by. Built concurrently, so
-- writes are never blocked for the build. Every row already there has a NULL
-- key, so the index starts empty. The drop ahead of the build clears an
-- invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_lead_company;
CREATE INDEX CONCURRENTLY idx_lead_company ON lead (company_id) WHERE company_id IS NOT NULL;
