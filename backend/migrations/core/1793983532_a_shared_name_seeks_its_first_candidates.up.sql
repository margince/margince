-- pgmigrate:no-transaction
-- Equal trigram sets score one. The id suffix bounds their deterministic tie order.
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_name_trigram_candidates;
CREATE INDEX CONCURRENTLY idx_contact_name_trigram_candidates ON contact
    (hash_array_extended(show_trgm(f_fold_apostrophes(lower(full_name))), 0), id)
    WHERE archived_at IS NULL;
