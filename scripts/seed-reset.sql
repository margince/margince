-- seed-reset.sql — clear the demo installation's RECORDS so `make seed-dev`
-- can fill it again. Run by `make seed-reset` against the compose stack's
-- Postgres.
--
-- What it deletes: every base table in `public` except the preserved set below.
-- What it keeps: the installation itself — its workspace, its people, its roles
-- and sessions, its configuration and its append-only ledgers — so the stack is
-- usable the moment this finishes. `make seed-dev` runs straight afterwards with
-- no restart, because the admin it logs in as is still there.
--
-- IT NO LONGER DELETES THE WORKSPACE. It used to, and the recovery then depended
-- on the API re-bootstrapping the organization from margince.yaml at its NEXT
-- boot — so a reset against a running stack left seed-dev with no workspace to
-- seed into and nothing saying why.
--
-- THE PRESERVED SET IS THE WHOLE DEFINITION, and it is the same one the
-- in-product data reset uses: internal/compose/datasweep.go's
-- preservedResetTables. backend/gates/seedresetparity_test.go holds the two
-- equal in both directions, because two answers to "what survives a reset" is
-- how a dev database and a customer's diverge quietly. The reasoning for each
-- entry lives there, next to the code that acts on it, rather than being
-- restated here where it would drift.
--
-- session_replication_role = replica disables FK enforcement and triggers for
-- the duration, so the deletes are order-independent. Requires superuser (the
-- compose stack's margince_owner is one).

BEGIN;

SET LOCAL session_replication_role = replica;

DO $$
DECLARE
  t       text;
  targets int;
  -- Mirrored from internal/compose/datasweep.go. Sorted, and held equal to it by
  -- backend/gates/seedresetparity_test.go — edit neither side alone.
  preserved text[] := ARRAY[
    'activity_kind',
    'activity_retention_evidence',
    'ai_call_config',
    'app_user',
    'audit_log',
    'auth_token',
    'channel_provider',
    'embed_store_binding',
    'event_outbox',
    'lead_disqualify_reason',
    'lead_source',
    'overlay_mode',
    'passport',
    'role',
    'role_assignment',
    'session',
    'setting',
    'system_log',
    'team',
    'team_membership',
    'vault_secret',
    'workspace'
  ];
BEGIN
  -- The corpus is asserted before anything is deleted. A derivation that
  -- silently found nothing would report success over an untouched database —
  -- under-recognition, and indistinguishable from a reset that worked.
  SELECT count(*) INTO targets
  FROM pg_class c
  JOIN pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname = 'public'
    AND c.relkind = 'r'
    AND c.relname NOT LIKE 'schema\_migrations%'
    AND c.relname NOT LIKE 'river\_%'
    AND NOT (c.relname = ANY(preserved));

  IF targets = 0 THEN
    RAISE EXCEPTION 'seed-reset: no table in public is a reset target — every one is either preserved, a migration ledger or River''s. That means the preserved list has grown to cover the whole schema, which is a mistake in the list rather than a database with nothing in it.';
  END IF;

  FOR t IN
    SELECT c.relname
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public'
      AND c.relkind = 'r'
      AND c.relname NOT LIKE 'schema\_migrations%'
      AND c.relname NOT LIKE 'river\_%'
      AND NOT (c.relname = ANY(preserved))
    ORDER BY c.relname
  LOOP
    EXECUTE format('DELETE FROM %I', t);
  END LOOP;

  RAISE NOTICE 'seed-reset: cleared % record table(s); the installation, its people and its configuration are untouched — run make seed-dev to fill it', targets;
END $$;

COMMIT;
