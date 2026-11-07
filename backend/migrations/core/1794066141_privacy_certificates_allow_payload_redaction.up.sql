SET LOCAL lock_timeout = '3s';

CREATE FUNCTION audit_redacted_evidence(certificate boolean, payload jsonb) RETURNS jsonb
LANGUAGE plpgsql IMMUTABLE AS $$DECLARE
  kept jsonb := '{"redacted":true}'::jsonb;
  field text;
  held jsonb;
  reduced jsonb;
BEGIN
  IF NOT certificate OR jsonb_typeof(payload) IS DISTINCT FROM 'object' THEN
    RETURN kept;
  END IF;
  FOR field, held IN SELECT key, value FROM jsonb_each(payload) LOOP
    IF field = 'rows' AND jsonb_typeof(held) = 'object' THEN
      SELECT coalesce(jsonb_object_agg(key, value), '{}'::jsonb) INTO reduced
      FROM jsonb_each(held) WHERE key ~ '^[a-z_][a-z_0-9]*\.(updated|deleted|inserted)$'
        AND jsonb_typeof(value) = 'number';
      kept := kept || jsonb_build_object(field, reduced);
    ELSIF field = 'uncounted' AND jsonb_typeof(held) = 'array' THEN
      SELECT coalesce(jsonb_agg(arm), '[]'::jsonb) INTO reduced
      FROM jsonb_array_elements_text(held) arm
      WHERE arm IN ('attachment_object','controller_delivery_payload','per_table_rows');
      kept := kept || jsonb_build_object(field, reduced);
    ELSIF field IN ('emails_suppressed','raw_rows_purged','ai_payloads_purged',
                    'activities_redacted','activities_restricted','channel_identities_suppressed','retain_days')
          AND jsonb_typeof(held) = 'number' THEN
      kept := kept || jsonb_build_object(field, held);
    ELSIF field IN ('cause','class','basis','retention_action','purge_reason','policy','request_id')
          AND jsonb_typeof(held) = 'string' THEN
      kept := kept || jsonb_build_object(field, held);
    ELSIF field = 'reason' AND (held #>> '{}') ~ '^dsr:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
          AND (held #>> '{}') <> 'dsr:00000000-0000-0000-0000-000000000000' THEN
      kept := kept || jsonb_build_object(field, held);
    END IF;
  END LOOP;
  RETURN kept;
END;
$$;

CREATE OR REPLACE FUNCTION audit_log_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'UPDATE'
     AND NEW.before IS NULL AND NEW.after IS NULL
     AND NEW.evidence = audit_redacted_evidence(
       OLD.action IN ('erase','anonymize','expire','release') AND OLD.before IS NULL AND OLD.after IS NULL, OLD.evidence)
     AND (to_jsonb(NEW) - ARRAY['before','after','evidence']) =
         (to_jsonb(OLD) - ARRAY['before','after','evidence'])
     AND EXISTS (
       SELECT 1 FROM audit_log certificate
       WHERE certificate.before IS NULL AND certificate.after IS NULL
         AND certificate.xmin::text = pg_current_xact_id()::text
         AND ((certificate.entity_type = OLD.entity_type
               AND certificate.entity_id = OLD.entity_id
               AND certificate.action IN ('erase','anonymize','expire','release'))
           OR (certificate.action = 'update' AND certificate.entity_type = 'audit_log'
               AND certificate.evidence = '{"payload_redaction":true}'::jsonb
               AND EXISTS (
                 SELECT 1 FROM audit_log boundary
                 WHERE boundary.id = certificate.entity_id
                   AND boundary.entity_type = OLD.entity_type AND boundary.entity_id = OLD.entity_id
                   AND boundary.action IN ('erase','anonymize','expire','release')
                   AND boundary.before IS NULL AND boundary.after IS NULL
                   AND (OLD.occurred_at, OLD.id) <= (boundary.occurred_at, boundary.id))))
     ) THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'audit metadata is immutable; payload redaction needs a current privacy certificate'
    USING ERRCODE = 'check_violation';
END;
$$;

GRANT UPDATE (before, after, evidence) ON audit_log TO margince_app;
