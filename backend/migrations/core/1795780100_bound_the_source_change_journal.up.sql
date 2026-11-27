SET LOCAL lock_timeout = '3s';

-- Each connection keeps a small indexed journal. Transaction and savepoint
-- rollback cover its rows, and commit empties it before the connection is reused.
CREATE OR REPLACE FUNCTION public.remember_a_triggered_source_change() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
BEGIN
    IF to_regclass('pg_temp.margince_source_changes') IS NULL THEN
        CREATE TEMP TABLE margince_source_changes (
            namespace text NOT NULL,
            entity_type text NOT NULL,
            entity_id uuid NOT NULL,
            change_id uuid NOT NULL DEFAULT public.uuidv7() UNIQUE,
            payload jsonb NOT NULL,
            claimed boolean NOT NULL DEFAULT false,
            PRIMARY KEY (namespace, entity_type, entity_id)
        ) ON COMMIT DELETE ROWS;
        CREATE INDEX ON margince_source_changes(namespace) WHERE NOT claimed;
    END IF;
    INSERT INTO pg_temp.margince_source_changes AS journal(namespace, entity_type, entity_id, payload)
    VALUES (TG_TABLE_SCHEMA, TG_TABLE_NAME, NEW.id, jsonb_build_object(
        'entity_type', TG_TABLE_NAME, 'id', NEW.id,
        'before_system', to_jsonb(OLD)->'source_system',
        'after_system', to_jsonb(NEW)->'source_system',
        'system_changed', (to_jsonb(OLD)->'source_system') IS DISTINCT FROM (to_jsonb(NEW)->'source_system'),
        'before', OLD.source, 'after', NEW.source, 'deal_id', to_jsonb(NEW)->'deal_id'))
    ON CONFLICT (namespace, entity_type, entity_id) DO UPDATE
    SET payload = journal.payload || jsonb_build_object(
        'after', NEW.source, 'after_system', to_jsonb(NEW)->'source_system',
        'system_changed', (journal.payload->>'system_changed')::boolean OR
            ((to_jsonb(OLD)->'source_system') IS DISTINCT FROM (to_jsonb(NEW)->'source_system')));
    RETURN NEW;
END;
$$;

-- namespace is current_schema() at the bound store call. The function cannot
-- see another connection's journal and never resolves an unqualified relation.
CREATE FUNCTION public.source_changes_for_audit(namespace text, kind text, identities uuid[])
RETURNS SETOF jsonb LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
BEGIN
    IF to_regclass('pg_temp.margince_source_changes') IS NULL THEN RETURN; END IF;
    IF kind IS NULL THEN
        RETURN QUERY SELECT payload || jsonb_build_object('change_id', journal.change_id) FROM pg_temp.margince_source_changes AS journal
            WHERE journal.namespace = source_changes_for_audit.namespace;
        RETURN;
    END IF;
    RETURN QUERY
        WITH claimed AS (
            UPDATE pg_temp.margince_source_changes AS journal SET claimed = true
            WHERE journal.namespace = source_changes_for_audit.namespace AND NOT journal.claimed
            RETURNING journal.payload || jsonb_build_object('change_id', journal.change_id) AS payload
        )
        SELECT payload FROM claimed
        UNION ALL
        SELECT payload || jsonb_build_object('change_id', journal.change_id) FROM pg_temp.margince_source_changes AS journal
        WHERE journal.namespace = source_changes_for_audit.namespace
          AND journal.entity_type = kind AND journal.entity_id = ANY(identities);
END;
$$;

CREATE FUNCTION public.consume_source_changes(namespace text, changes uuid[])
RETURNS void LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
BEGIN
    IF to_regclass('pg_temp.margince_source_changes') IS NULL THEN RETURN; END IF;
    DELETE FROM pg_temp.margince_source_changes AS journal
    WHERE journal.namespace = consume_source_changes.namespace
      AND (changes IS NULL OR journal.change_id = ANY(changes));
END;
$$;
