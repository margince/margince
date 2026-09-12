-- One-time database bootstrap. Run ONCE against your Postgres as a superuser
-- BEFORE the first api deploy, then never again. For example:
--
--   psql "postgres://postgres:…@<host>:5432/postgres" \
--     -v owner_pw="$OWNER_PW" -v app_pw="$APP_PW" -f scripts/deploy/db-bootstrap.sql
--
-- Pass the two role passwords RAW (not pre-quoted) as psql variables so they
-- never land in this committed file — `:'…'` + `%L` quote and escape them. They
-- MUST match the passwords embedded in the app's MARGINCE_OWNER_DSN / MARGINCE_DSN.
--
-- Why two non-superuser roles: the runtime holds DML grants only, and a
-- superuser ignores every grant — so the wall between what serves traffic and
-- what applies DDL exists only while neither role is exempt. The api refuses to
-- serve on an exempt runtime role (compose.AssertRuntimeRole).
--   * margince_owner — owns the database + tables, runs migrations (DDL). Not a
--     superuser, no BYPASSRLS.
--   * margince_app   — the runtime role the api/worker connect as. Its table
--     grants are applied by migration 0015_app_role_grants, which is a no-op
--     unless the role already exists — hence it is created here, first.
--
-- Idempotent: safe to re-run (each step guards on existence).

\set ON_ERROR_STOP on

-- The two roles. psql does NOT interpolate `:'var'` inside a dollar-quoted
-- DO $$…$$ body, so the guarded CREATE ROLE is built in a plain SELECT (where
-- interpolation DOES happen) and run with \gexec. `format(… %L …)` safely quotes
-- the password; the WHERE NOT EXISTS makes each idempotent (an existing role
-- yields no row, so \gexec runs nothing). Neither role is a superuser or granted
-- BYPASSRLS — their grants must bind them.

-- The runtime app role (mirrors scripts/db-init.sql for local dev).
SELECT format('CREATE ROLE margince_app LOGIN PASSWORD %L', :'app_pw')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'margince_app')
\gexec

-- The owner role that runs migrations and owns every object.
SELECT format('CREATE ROLE margince_owner LOGIN PASSWORD %L', :'owner_pw')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'margince_owner')
\gexec

-- Normalize the security-critical attributes UNCONDITIONALLY. The NOT EXISTS
-- guards above skip a role that already exists, so a pre-existing margince_app /
-- margince_owner could otherwise retain SUPERUSER or BYPASSRLS and silently
-- ignore its own grants. These ALTERs are idempotent and cost nothing on a fresh role
-- (CREATE ROLE already defaults to NOSUPERUSER NOBYPASSRLS).
ALTER ROLE margince_app   NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
ALTER ROLE margince_owner NOSUPERUSER NOBYPASSRLS;

-- Reassigning ownership below needs the connecting role (RDS's master user
-- on the real path; whoever runs this script on any other Postgres) to be a
-- MEMBER of margince_owner first — plain PostgreSQL rule for ALTER
-- .../OWNER TO: the executor must already own the object AND be a member of
-- the new owning role, unless the executor is a true superuser. RDS's master
-- user holds rds_superuser, not superuser (see
-- docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Appendix.PostgreSQL.CommonDBATasks.Roles.rds_superuser.html),
-- so it is not exempt, and CREATE ROLE above did not make it a member of the
-- role it just created — that membership has to be granted explicitly.
-- INHERIT (the default) means everything below runs with margince_owner's
-- privileges without a SET ROLE. Revoked again at the end of this script,
-- once nothing further needs it.
GRANT margince_owner TO CURRENT_USER;

-- The application database, owned by margince_owner. CREATE DATABASE cannot run
-- inside a DO block or a transaction, so it is guarded with \gexec instead.
--
-- The OWNER clause above only takes effect on the branch that actually
-- creates the database. On RDS (deploy/terraform/aws/rds.tf sets
-- `db_name = "margince"`), the database already exists — created by RDS
-- itself, owned by the master user — before this script ever runs, so the
-- WHERE NOT EXISTS guard skips this line and margince_owner never owns
-- anything on that (the overwhelmingly common) path. The ALTER DATABASE
-- below is unconditional and idempotent — a no-op when this script did just
-- create it, the actual fix when RDS did.
SELECT 'CREATE DATABASE margince OWNER margince_owner'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'margince')
\gexec
ALTER DATABASE margince OWNER TO margince_owner;

-- The app role must be able to reach the database; object-level grants come from
-- migration 0015 (run by the api entrypoint as margince_owner).
GRANT CONNECT ON DATABASE margince TO margince_app;

-- Extensions the migrations expect. `vector` (pgvector) is NOT a trusted
-- extension, so it cannot be installed by the non-superuser owner from a
-- migration — pre-install it (and the trusted ones too, so every migration's
-- `CREATE EXTENSION IF NOT EXISTS` is a guaranteed no-op) here as superuser.
\connect margince

-- Same reasoning as the database ALTER above: `public` is created by
-- initdb, not by this script, so a fresh Postgres ≥15 (RDS 16 included —
-- see rds.tf) has already revoked CREATE on it from everyone but its owner
-- before this script runs. Migration 0001_baseline's very first statement
-- (CREATE SCHEMA ext) and its CREATE EXTENSION calls both need margince_owner
-- to hold that privilege, which owning the schema outright guarantees
-- without a separate GRANT to maintain in step. Covered by the same
-- membership grant taken out above — this is a second object, not a second
-- permission requirement.
ALTER SCHEMA public OWNER TO margince_owner;

CREATE EXTENSION IF NOT EXISTS vector;      -- 0022_embeddings (pgvector; untrusted)
CREATE EXTENSION IF NOT EXISTS unaccent;    -- 0052_fts_linguistics
CREATE EXTENSION IF NOT EXISTS pg_trgm;     -- 0052_fts_linguistics
CREATE EXTENSION IF NOT EXISTS btree_gist;  -- 0032_meeting_exclusion

-- Drop the membership taken out above — nothing past this point needs it,
-- and margince_owner's privileges have no business lingering on whatever
-- role runs this script after it exits.
REVOKE margince_owner FROM CURRENT_USER;
