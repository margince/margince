-- One-time cluster setup (run by `make db-init` as the database owner):
-- the runtime role the server connects as. Non-superuser, no BYPASSRLS,
-- not the table owner — its grants bind it with no exception path.

-- The roles below are cluster-wide, so one run at a time changes them: a
-- session lock, held until this psql session ends. An advisory lock is per
-- database, so every caller runs this script in the `postgres` database.
DO $$ BEGIN PERFORM pg_advisory_lock(7300148210); END $$;
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'margince_app') THEN
    CREATE ROLE margince_app LOGIN PASSWORD 'margince_app_dev';
  END IF;
END $$;

-- The role a unit's own SQL runs as (Runtime.Tx): DML on schema ext, nothing on
-- public. NOLOGIN, because the runtime role reaches it by switching to it inside
-- a transaction, never by connecting.
DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ext_runtime') THEN
    CREATE ROLE ext_runtime NOLOGIN NOINHERIT;
  END IF;
END $$;
ALTER ROLE ext_runtime NOLOGIN NOSUPERUSER NOINHERIT NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
GRANT ext_runtime TO margince_app;
