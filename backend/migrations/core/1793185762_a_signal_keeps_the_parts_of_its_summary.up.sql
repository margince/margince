-- The parts a server-worded summary was built from, so a reader sees it in
-- their own language. NULL on a signal whose summary was not built from parts.
--
-- Nullable with no default and no backfill, so adding it rewrites no row.
SET LOCAL lock_timeout = '3s';

ALTER TABLE signal ADD COLUMN summary_params jsonb;
