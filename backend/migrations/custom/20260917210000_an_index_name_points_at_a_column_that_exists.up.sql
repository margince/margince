-- `idx_import_run_ws` describes a column no table has.
--
-- The `ws` segment is `workspace` — the tenant column ADR-0091 §8 phase D
-- dropped from every table in this schema. An index name reaches a reader
-- through EXPLAIN output and through the pg_stat_user_indexes row an operator
-- reads when deciding whether an index earns its keep, and this one sends them
-- looking for a column that does not exist.
--
-- Here rather than beside the eight core renames it belongs with: this index is
-- created by THIS namespace, and a core migration may not assume this one has
-- run.
SET LOCAL lock_timeout = '3s';

ALTER INDEX idx_import_run_ws RENAME TO idx_import_run_status;
