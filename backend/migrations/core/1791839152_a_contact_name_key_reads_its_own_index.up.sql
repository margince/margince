-- pgmigrate:no-transaction
-- The contact dedupe candidate set ORs the trigram arm with an exact-name-key
-- arm (contacts exactNameKeySQL). An OR is read from indexes only when every
-- arm has one, so without this index each dedupe read the whole contact table,
-- and an import of N contacts cost N table scans.
--
-- Built concurrently, so contact writes are never blocked for the build. The
-- drop ahead of the build clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_exact_name_key;
CREATE INDEX CONCURRENTLY idx_contact_exact_name_key ON contact (btrim(regexp_replace(replace(f_fold_apostrophes(lower(full_name)), 'ς', 'σ'), '\s+', ' ', 'g'))) WHERE archived_at IS NULL;
