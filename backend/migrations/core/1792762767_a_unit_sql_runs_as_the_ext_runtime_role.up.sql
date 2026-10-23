-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- What a unit's own SQL may touch, as a grant instead of a convention.
--
-- Runtime.Tx runs each of a unit's statements as ext_runtime: DML on the tables
-- in schema ext and nothing on public, so a statement that names a core table
-- is answered permission denied by the database. One role for every unit, not
-- one per unit: a unit can still reach another unit's ext tables.
--
-- The role itself is cluster-level, made beside margince_app (scripts/db-init.sql
-- and the deploy bootstrap). The migration refuses to run without it: a skipped
-- grant would leave every unit table refused at run time, with nothing to say
-- why, and an applied version never runs again.
--
-- Tables a unit's migration creates later are covered by the default privileges:
-- they apply to what THIS role, the one that applies unit migrations, creates in
-- schema ext. A unit's migration grants nothing to ext_runtime itself, and the
-- extension migration gate refuses a grant to any role but margince_app.
SET LOCAL lock_timeout = '3s';

DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ext_runtime') THEN
    RAISE EXCEPTION 'the ext_runtime role does not exist: create it first (make db-init, or scripts/deploy/db-bootstrap.sql) and run the migration again';
  END IF;
  GRANT USAGE ON SCHEMA ext TO ext_runtime;
  GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA ext TO ext_runtime;
  GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA ext TO ext_runtime;
  ALTER DEFAULT PRIVILEGES IN SCHEMA ext GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ext_runtime;
  ALTER DEFAULT PRIVILEGES IN SCHEMA ext GRANT USAGE, SELECT ON SEQUENCES TO ext_runtime;
END $$;
