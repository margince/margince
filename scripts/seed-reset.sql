-- seed-reset.sql — wipe the demo installation's workspace so
-- `make seed-dev` can rebuild it from scratch. Run by `make seed-reset`
-- against the compose stack's Postgres.
--
-- Deletes every row scoped to the demo workspace across all tenant tables
-- (those with a workspace_id column), discovered dynamically so a new
-- table is covered without touching this file.
--
-- session_replication_role = replica disables FK enforcement and triggers
-- for the duration, so the deletes are order-independent. That includes
-- audit_log's append-only guard — correct here, because the reset erases
-- the whole tenant, history included. Requires superuser (the compose
-- stack's margince_owner is one).

BEGIN;

SET LOCAL session_replication_role = replica;

DO $$
DECLARE
  ws uuid;
  t  text;
BEGIN
  -- The installation's one workspace (ADR-0061); ADR-0091 retired the slug this
  -- used to match on. INTO STRICT, not LIMIT 1, and the distinction matters more
  -- here than anywhere: this block DELETES. Picking whichever workspace happened
  -- to be oldest would wipe a tenant nobody named.
  BEGIN
    SELECT id INTO STRICT ws FROM workspace WHERE archived_at IS NULL;
  EXCEPTION
    WHEN no_data_found THEN
      RAISE NOTICE 'seed-reset: no live workspace — nothing to do';
      RETURN;
    WHEN too_many_rows THEN
      RAISE EXCEPTION 'seed-reset: more than one live workspace — refusing to delete, since there is no such thing as THE demo installation here';
  END;

  FOR t IN
    SELECT c.table_name
    FROM information_schema.columns c
    JOIN information_schema.tables tb
      ON tb.table_schema = c.table_schema AND tb.table_name = c.table_name
    WHERE c.table_schema = 'public'
      AND c.column_name = 'workspace_id'
      AND tb.table_type = 'BASE TABLE'
  LOOP
    EXECUTE format('DELETE FROM %I WHERE workspace_id = %L', t, ws);
  END LOOP;

  DELETE FROM workspace WHERE id = ws;
END $$;

COMMIT;
