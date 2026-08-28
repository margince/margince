-- seed-reset.sql — wipe the demo installation's workspace so
-- `make seed-dev` can rebuild it from scratch. Run by `make seed-reset`
-- against the compose stack's Postgres.
--
-- Deletes every row scoped to the demo workspace across all tenant tables
-- (those with a workspace_id column), discovered dynamically so a new
-- table is covered without touching this file.
--
-- THE DISCOVERY IS ALSO THE GUARD. This script's last statement removes the
-- workspace row itself, and session_replication_role = replica means no FK
-- cascade follows it — so a loop that finds nothing does not merely delete
-- nothing, it leaves every domain row pointing at a workspace that is gone.
-- Silent under-recognition, and destructive. The count is therefore asserted
-- before anything is deleted: an empty target set aborts the transaction and
-- says what has to be decided, rather than half-erasing an installation and
-- reporting success.
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
  t       text;
  targets int;
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

  SELECT count(*) INTO targets
  FROM information_schema.columns c
  JOIN information_schema.tables tb
    ON tb.table_schema = c.table_schema AND tb.table_name = c.table_name
  WHERE c.table_schema = 'public'
    AND c.column_name = 'workspace_id'
    AND tb.table_type = 'BASE TABLE';

  IF targets = 0 THEN
    RAISE EXCEPTION 'seed-reset: no table in public carries workspace_id, so this script would delete the workspace row and nothing else — leaving every domain row orphaned against a workspace that is gone. Deciding what a full reset KEEPS (the migration-seeded reference data an installation cannot re-create: setting, channel_provider, activity_kind, lead_source, lead_disqualify_reason) and what it erases is a decision this script may not guess. Until it is made, wipe the stack instead: make dev-stop && make db-up && make migrate && make seed-dev.';
  END IF;

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
