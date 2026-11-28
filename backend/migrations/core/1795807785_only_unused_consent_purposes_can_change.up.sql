SET LOCAL lock_timeout = '3s';

CREATE FUNCTION public.normalized_consent_purpose_key(input text) RETURNS text
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE SET search_path = pg_catalog, public AS $$
    SELECT lower(btrim(input, U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000'));
$$;

CREATE FUNCTION public.consent_purpose_keys_in(document jsonb) RETURNS text[]
LANGUAGE sql IMMUTABLE PARALLEL SAFE SET search_path = pg_catalog, public AS $$
    SELECT coalesce(array_agg(DISTINCT public.normalized_consent_purpose_key(value #>> '{}'))
                    FILTER (WHERE jsonb_typeof(value) = 'string'), ARRAY[]::text[])
    FROM (
        SELECT jsonb_path_query(document, '$.**.consent_purpose') AS value
        UNION ALL
        SELECT jsonb_path_query(document, '$.**.marketing_purpose') AS value
    ) keys;
$$;

CREATE FUNCTION public.consent_purpose_has_references(target uuid, purpose_key text) RETURNS boolean
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
DECLARE reference record; used boolean;
BEGIN
    FOR reference IN
        SELECT ns.nspname AS schema_name, rel.relname AS table_name, col.attname AS column_name
        FROM pg_constraint fk
        JOIN pg_class rel ON rel.oid = fk.conrelid
        JOIN pg_namespace ns ON ns.oid = rel.relnamespace
        JOIN pg_attribute col ON col.attrelid = fk.conrelid AND col.attnum = ANY(fk.conkey)
        WHERE fk.contype = 'f' AND fk.confrelid = 'public.consent_purpose'::regclass
    LOOP
        EXECUTE format('SELECT EXISTS (SELECT 1 FROM %I.%I WHERE %I = $1)',
                       reference.schema_name, reference.table_name, reference.column_name)
            INTO used USING target;
        IF used THEN RETURN true; END IF;
    END LOOP;
    RETURN EXISTS (SELECT 1 FROM public.consent_text_version WHERE public.normalized_consent_purpose_key(key) = public.normalized_consent_purpose_key($2))
        OR EXISTS (SELECT 1 FROM public.communication_decision WHERE public.normalized_consent_purpose_key(communication_decision.purpose_key) = public.normalized_consent_purpose_key($2))
        OR EXISTS (SELECT 1 FROM public.comms_outbound WHERE public.normalized_consent_purpose_key(consent_purpose) = public.normalized_consent_purpose_key($2))
        OR EXISTS (SELECT 1 FROM public.scheduled_send
                   WHERE public.normalized_consent_purpose_key($2) = ANY(public.consent_purpose_keys_in(payload)))
        OR EXISTS (SELECT 1 FROM public.automation
                   WHERE public.normalized_consent_purpose_key($2) = ANY(public.consent_purpose_keys_in(jsonb_build_array(action, params))))
        OR EXISTS (SELECT 1 FROM public.approval
                   WHERE public.normalized_consent_purpose_key($2) = ANY(public.consent_purpose_keys_in(proposed_change)));
END;
$$;

CREATE FUNCTION public.refuse_a_used_purpose_change() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
BEGIN
    IF NEW.label IS NOT DISTINCT FROM OLD.label AND NEW.key IS NOT DISTINCT FROM OLD.key
       AND NEW.archived_at IS NOT DISTINCT FROM OLD.archived_at THEN RETURN NEW; END IF;
    PERFORM 1 FROM public.consent_purpose WHERE id = OLD.id FOR UPDATE;
    IF public.consent_purpose_has_references(OLD.id, OLD.key) THEN
        RAISE EXCEPTION 'consent is already recorded for this purpose'
            USING ERRCODE = '23514', CONSTRAINT = 'consent_purpose_in_use';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER used_purpose_does_not_change BEFORE UPDATE OF key, label, archived_at ON public.consent_purpose
FOR EACH ROW EXECUTE FUNCTION public.refuse_a_used_purpose_change();

CREATE FUNCTION public.require_a_live_consent_purpose_reference() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
DECLARE target uuid; retired boolean;
BEGIN
    target := (to_jsonb(NEW)->>TG_ARGV[0])::uuid;
    IF target IS NULL THEN RETURN NEW; END IF;
    IF TG_OP = 'UPDATE' AND (to_jsonb(OLD)->>TG_ARGV[0])::uuid IS NOT DISTINCT FROM target THEN RETURN NEW; END IF;
    SELECT archived_at IS NOT NULL INTO retired FROM public.consent_purpose WHERE id = target FOR KEY SHARE;
    IF retired THEN
        RAISE EXCEPTION 'consent purpose is archived'
            USING ERRCODE = '23514', CONSTRAINT = 'consent_purpose_in_use';
    END IF;
    RETURN NEW;
END;
$$;

DO $$
DECLARE reference record;
BEGIN
    FOR reference IN
        SELECT DISTINCT ns.nspname AS schema_name, rel.relname AS table_name, col.attname AS column_name
        FROM pg_constraint fk
        JOIN pg_class rel ON rel.oid = fk.conrelid
        JOIN pg_namespace ns ON ns.oid = rel.relnamespace
        JOIN pg_attribute col ON col.attrelid = fk.conrelid AND col.attnum = ANY(fk.conkey)
        WHERE fk.contype = 'f' AND fk.confrelid = 'public.consent_purpose'::regclass
    LOOP
        EXECUTE format('CREATE TRIGGER live_consent_purpose_reference BEFORE INSERT OR UPDATE OF %I ON %I.%I FOR EACH ROW EXECUTE FUNCTION public.require_a_live_consent_purpose_reference(%L)',
            reference.column_name, reference.schema_name, reference.table_name, reference.column_name);
    END LOOP;
END;
$$;

CREATE FUNCTION public.lock_consent_purpose_keys() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, public AS $$
DECLARE keys text[]; item record; document jsonb; column_name text; refusal boolean := false;
BEGIN
    IF TG_ARGV[0] = 'json' THEN
        document := '[]'::jsonb;
        FOREACH column_name IN ARRAY string_to_array(TG_ARGV[2], ',') LOOP
            document := document || jsonb_build_array(to_jsonb(NEW)->column_name);
        END LOOP;
        keys := public.consent_purpose_keys_in(document);
    ELSE
        keys := ARRAY[public.normalized_consent_purpose_key(to_jsonb(NEW)->>TG_ARGV[0])];
    END IF;
    IF TG_ARGV[1] = 'decision' THEN
        refusal := NEW.verdict IN ('deny', 'review');
    END IF;
    FOR item IN SELECT id, archived_at FROM public.consent_purpose WHERE public.normalized_consent_purpose_key(key) = ANY(keys) ORDER BY key FOR KEY SHARE LOOP
        IF item.archived_at IS NOT NULL AND NOT refusal THEN
            RAISE EXCEPTION 'consent purpose is archived'
                USING ERRCODE = '23514', CONSTRAINT = 'consent_purpose_in_use';
        END IF;
    END LOOP;
    RETURN NEW;
END;
$$;
CREATE TRIGGER consent_purpose_keys BEFORE INSERT OR UPDATE OF payload ON public.scheduled_send
FOR EACH ROW EXECUTE FUNCTION public.lock_consent_purpose_keys('json', 'live', 'payload');
CREATE TRIGGER consent_purpose_keys BEFORE INSERT ON public.communication_decision
FOR EACH ROW EXECUTE FUNCTION public.lock_consent_purpose_keys('purpose_key', 'decision');
CREATE TRIGGER consent_purpose_keys BEFORE INSERT ON public.comms_outbound
FOR EACH ROW EXECUTE FUNCTION public.lock_consent_purpose_keys('consent_purpose', 'live');
CREATE TRIGGER consent_purpose_keys BEFORE INSERT OR UPDATE OF key ON public.consent_text_version
FOR EACH ROW EXECUTE FUNCTION public.lock_consent_purpose_keys('key', 'live');

CREATE TRIGGER consent_purpose_keys BEFORE INSERT OR UPDATE OF action, params ON public.automation
FOR EACH ROW EXECUTE FUNCTION public.lock_consent_purpose_keys('json', 'live', 'action,params');
CREATE TRIGGER consent_purpose_keys BEFORE INSERT OR UPDATE OF proposed_change ON public.approval
FOR EACH ROW EXECUTE FUNCTION public.lock_consent_purpose_keys('json', 'live', 'proposed_change');
