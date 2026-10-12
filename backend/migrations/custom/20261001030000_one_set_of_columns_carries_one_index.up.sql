-- import_run's share of the duplicate-index sweep the core namespace does for
-- the upstream tables. uq_import_run was UNIQUE (workspace_id, id); when
-- workspace_id was dropped it collapsed to UNIQUE (id), which import_run_pkey
-- already enforces. It is here rather than beside the other nine because this
-- table is the fork's own and core may not name it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE import_run DROP CONSTRAINT uq_import_run;
