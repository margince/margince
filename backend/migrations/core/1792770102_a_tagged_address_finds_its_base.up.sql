-- pgmigrate:no-transaction
-- Two indexes for the contact duplicate check, so neither new arm reads the
-- whole table on every create.
--
--   - idx_contact_email_base serves the address-variant lane, which pairs an
--     address with the same address under a plus tag (contacts addressBaseSQL).
--   - idx_contact_exact_digraph_name_key serves the candidate query's arm that
--     matches "Björn" to "Bjoern" (contacts exactDigraphNameKeySQL).
--
-- Each expression is spelled exactly as the query spells it, because Postgres
-- pairs an expression index with a query by syntax. Built concurrently, so
-- contact writes are never blocked. Each drop clears an invalid index a failed
-- build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_email_base;
CREATE INDEX CONCURRENTLY idx_contact_email_base ON contact_email (regexp_replace(lower(email), '^([^+@]+)\+[^@]*@', '\1@')) WHERE archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_exact_digraph_name_key;
CREATE INDEX CONCURRENTLY idx_contact_exact_digraph_name_key ON contact (btrim(regexp_replace(replace(f_fold_apostrophes(lower(replace(replace(replace(normalize(lower(full_name), NFC), 'ä', 'ae'), 'ö', 'oe'), 'ü', 'ue'))), 'ς', 'σ'), '\s+', ' ', 'g'))) WHERE archived_at IS NULL;
