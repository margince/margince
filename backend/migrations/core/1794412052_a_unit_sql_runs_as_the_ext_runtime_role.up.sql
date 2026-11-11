-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- What a unit's own SQL may touch, as a grant instead of a convention.
--
-- Runtime.Tx runs each of a unit's statements as ext_runtime: DML on the tables
-- in schema ext and nothing on public, so a statement that names a core table
-- is answered permission denied by the database. One role for every unit, not
-- one per unit: a unit can still reach another unit's ext tables.
--
-- The role is cluster-level. A migrator that may create roles (the dev and test
-- clusters) makes it here; a deployed owner may not, so the bootstrap makes it
-- (scripts/deploy/db-bootstrap.sql). Without it the migration refuses: a
-- skipped grant would leave every unit table refused at run time, with nothing
-- to say why, and an applied version never runs again.
--
-- A role that already exists is not trusted. A superuser migrator resets an
-- attribute that is wrong; any other migrator refuses it. Every migrator then
-- checks the role's memberships and its rights outside schema ext, and refuses
-- a role that holds more than it should.
--
-- The reset runs only when an attribute is wrong. ALTER ROLE rewrites the
-- role's row in pg_authid, which every database of the cluster shares, so two
-- databases migrating at once would both write that row and one would fail
-- with "tuple concurrently updated".
--
-- Tables a unit's migration creates later are covered by the default privileges:
-- they apply to what THIS role, the one that applies unit migrations, creates in
-- schema ext. A unit's migration grants nothing to ext_runtime itself.
SET LOCAL lock_timeout = '3s';

DO $$
DECLARE
  migrator_is_superuser boolean := (SELECT rolsuper FROM pg_roles WHERE rolname = current_user);
  may_create_roles boolean := (SELECT rolsuper OR rolcreaterole FROM pg_roles WHERE rolname = current_user);
  wrong_attribute boolean;
  found text;
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ext_runtime') THEN
    IF NOT may_create_roles THEN
      RAISE EXCEPTION 'the ext_runtime role does not exist. As a superuser, run: CREATE ROLE ext_runtime NOLOGIN NOINHERIT; GRANT ext_runtime TO margince_app; then run the migrations again';
    END IF;
    CREATE ROLE ext_runtime NOLOGIN NOINHERIT;
  END IF;

  SELECT rolcanlogin OR rolsuper OR rolinherit OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls
    INTO wrong_attribute FROM pg_roles WHERE rolname = 'ext_runtime';
  IF wrong_attribute AND migrator_is_superuser THEN
    ALTER ROLE ext_runtime NOLOGIN NOSUPERUSER NOINHERIT NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
  ELSIF wrong_attribute THEN
    RAISE EXCEPTION 'the ext_runtime role holds an attribute it must not have. As a superuser, run: ALTER ROLE ext_runtime NOLOGIN NOSUPERUSER NOINHERIT NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS; then run the migrations again';
  END IF;
  SELECT string_agg(granted.rolname, ', ') INTO found
    FROM pg_auth_members m JOIN pg_roles granted ON granted.oid = m.roleid
   WHERE m.member = 'ext_runtime'::regrole;
  IF found IS NOT NULL THEN
    RAISE EXCEPTION 'the ext_runtime role is a member of % and holds what that role holds. As a superuser, revoke the membership and run the migrations again', found;
  END IF;
  SELECT string_agg(format('%I.%I', n.nspname, c.relname), ', ') INTO found
    FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
   WHERE c.relkind IN ('r', 'p', 'v', 'm', 'f')
     AND n.nspname NOT IN ('ext', 'pg_catalog', 'information_schema') AND n.nspname NOT LIKE 'pg\_toast%'
     AND has_table_privilege('ext_runtime', c.oid, 'SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER');
  IF found IS NOT NULL THEN
    RAISE EXCEPTION 'the ext_runtime role holds rights on tables outside schema ext: %. As a superuser, revoke them and run the migrations again', found;
  END IF;

  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'margince_app')
     AND NOT pg_has_role('margince_app', 'ext_runtime', 'MEMBER') THEN
    IF NOT may_create_roles THEN
      RAISE EXCEPTION 'margince_app may not switch to the ext_runtime role. As a superuser, run: GRANT ext_runtime TO margince_app; then run the migrations again';
    END IF;
    GRANT ext_runtime TO margince_app;
  END IF;
  GRANT USAGE ON SCHEMA ext TO ext_runtime;
  GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA ext TO ext_runtime;
  GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA ext TO ext_runtime;
  ALTER DEFAULT PRIVILEGES IN SCHEMA ext GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ext_runtime;
  ALTER DEFAULT PRIVILEGES IN SCHEMA ext GRANT USAGE, SELECT ON SEQUENCES TO ext_runtime;
END $$;
