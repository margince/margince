-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_company_domain_disposition_candidate;
CREATE INDEX CONCURRENTLY idx_company_domain_disposition_candidate ON company_domain_disposition (candidate_company_id)
    WHERE candidate_company_id IS NOT NULL;
