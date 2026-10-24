-- pgmigrate:no-transaction
-- The company dedupe candidate set ORs its trigram arms with exact-name-key
-- arms on both name columns (contacts companyExactNameArms). An OR is read from
-- indexes only when every arm has one, so without these each company dedupe
-- read the whole company table, and the duplicate scan over N companies cost N
-- table scans.
--
-- The (updated_at, id) indexes serve the nightly duplicate scan, which pages
-- each of these tables in that order from its watermark.
--
-- Built concurrently, so writes are never blocked for the build. The drops
-- ahead of the builds clear an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_company_exact_name_key;
CREATE INDEX CONCURRENTLY idx_company_exact_name_key ON company (btrim(regexp_replace(replace(f_fold_apostrophes(lower(display_name)), 'ς', 'σ'), '\s+', ' ', 'g'))) WHERE archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_company_exact_legal_name_key;
CREATE INDEX CONCURRENTLY idx_company_exact_legal_name_key ON company (btrim(regexp_replace(replace(f_fold_apostrophes(lower(legal_name)), 'ς', 'σ'), '\s+', ' ', 'g'))) WHERE archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_company_dedupe_scan;
CREATE INDEX CONCURRENTLY idx_company_dedupe_scan ON company (updated_at, id) WHERE archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_dedupe_scan;
CREATE INDEX CONCURRENTLY idx_contact_dedupe_scan ON contact (updated_at, id) WHERE archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_company_domain_dedupe_scan;
CREATE INDEX CONCURRENTLY idx_company_domain_dedupe_scan ON company_domain (updated_at, id);
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_email_dedupe_scan;
CREATE INDEX CONCURRENTLY idx_contact_email_dedupe_scan ON contact_email (updated_at, id);
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_phone_dedupe_scan;
CREATE INDEX CONCURRENTLY idx_contact_phone_dedupe_scan ON contact_phone (updated_at, id);
