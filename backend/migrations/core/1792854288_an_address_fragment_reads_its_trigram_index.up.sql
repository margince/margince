-- pgmigrate:no-transaction
-- A fragment of an address or domain is searched through storekit.Identifier's
-- contains arm, which matches f_fold_apostrophes(lower(<column>)) over live rows.
-- Each trigram index holds that same expression and predicate, or every such
-- search reads the whole table.
--
-- Built concurrently so writes are never blocked. The drop ahead of each build
-- clears an invalid index a failed build left behind, so every statement is safe
-- to run again.
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_email_trgm;
CREATE INDEX CONCURRENTLY idx_contact_email_trgm ON contact_email USING gin (f_fold_apostrophes(lower(email)) gin_trgm_ops) WHERE archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_company_domain_trgm;
CREATE INDEX CONCURRENTLY idx_company_domain_trgm ON company_domain USING gin (f_fold_apostrophes(lower(domain)) gin_trgm_ops) WHERE archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_lead_email_trgm;
CREATE INDEX CONCURRENTLY idx_lead_email_trgm ON lead USING gin (f_fold_apostrophes(lower(email)) gin_trgm_ops) WHERE archived_at IS NULL;
