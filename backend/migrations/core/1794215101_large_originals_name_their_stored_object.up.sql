SET LOCAL lock_timeout = '3s';

ALTER TABLE raw_capture ADD COLUMN payload_object jsonb;
ALTER TABLE raw_capture ADD COLUMN payload_offload_checked_at timestamptz;
ALTER TABLE raw_capture ADD CONSTRAINT raw_capture_payload_object_shape CHECK (
    payload_object IS NULL OR (
        jsonb_typeof(payload_object) = 'object'
        AND jsonb_typeof(payload_object->'key') = 'string'
        AND jsonb_typeof(payload_object->'sha256') = 'string'
        AND (payload_object->>'sha256') ~ '^[0-9a-f]{64}$'
        AND jsonb_typeof(payload_object->'bytes') = 'number'
        AND (payload_object->>'bytes')::numeric > 0
    ) IS TRUE
) NOT VALID;
CREATE FUNCTION record_stored_object_intent(requested_kind text, requested_key text)
RETURNS void LANGUAGE sql AS $$INSERT INTO stored_object_intent (storage_key, kind)
    VALUES (requested_key, requested_kind)
    ON CONFLICT (storage_key) DO UPDATE
       SET kind = excluded.kind, recorded_at = now();
$$;

CREATE FUNCTION retire_raw_original_object()
RETURNS trigger LANGUAGE plpgsql AS $$BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF OLD.payload_object IS NOT NULL
           AND OLD.payload_object->>'key' = NEW.payload_object->>'key'
           AND (OLD.payload IS DISTINCT FROM NEW.payload
                OR (OLD.payload_object - 'matched_subjects') IS DISTINCT FROM (NEW.payload_object - 'matched_subjects')) THEN
            RAISE EXCEPTION 'A stored original rewrite requires a new object reference';
        END IF;
        IF OLD.payload_object IS NOT NULL
           AND (OLD.payload_object->>'key') IS DISTINCT FROM (NEW.payload_object->>'key') THEN
            PERFORM record_stored_object_intent('raw_original', OLD.payload_object->>'key');
        END IF;
        RETURN NEW;
    END IF;
    IF OLD.payload_object IS NOT NULL THEN
        PERFORM record_stored_object_intent('raw_original', OLD.payload_object->>'key');
    END IF;
    RETURN OLD;
END;
$$;

CREATE TRIGGER raw_original_object_retired
BEFORE DELETE OR UPDATE OF payload, payload_object ON raw_capture
FOR EACH ROW EXECUTE FUNCTION retire_raw_original_object();

GRANT EXECUTE ON FUNCTION record_stored_object_intent(text, text) TO margince_app;
