SET LOCAL lock_timeout = '3s';
DROP FUNCTION public.consume_source_changes(text, uuid[]);
DROP FUNCTION public.source_changes_for_audit(text, text, uuid[]);
SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION public.remember_a_triggered_source_change() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
DECLARE pending jsonb; identity text; prior jsonb;
BEGIN
    pending := coalesce(nullif(current_setting('margince.source_changes', true), ''), '{}')::jsonb;
    identity := TG_TABLE_NAME || ':' || NEW.id::text;
    prior := pending->identity;
    pending := jsonb_set(pending, ARRAY[identity], jsonb_build_object(
        'entity_type', TG_TABLE_NAME, 'id', NEW.id,
        'change_id', coalesce(prior->'change_id', to_jsonb(public.uuidv7())),
        'before_system', CASE WHEN prior IS NULL THEN to_jsonb(OLD)->'source_system' ELSE prior->'before_system' END,
        'after_system', to_jsonb(NEW)->'source_system',
        'system_changed', coalesce((prior->>'system_changed')::boolean,false) OR ((to_jsonb(OLD)->'source_system') IS DISTINCT FROM (to_jsonb(NEW)->'source_system')),
        'before', coalesce(prior->'before', to_jsonb(OLD.source)), 'after', NEW.source,
        'deal_id', to_jsonb(NEW)->'deal_id'));
    PERFORM set_config('margince.source_changes', pending::text, true);
    RETURN NEW;
END;
$$;
