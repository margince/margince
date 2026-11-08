SET LOCAL lock_timeout = '3s';
-- OR REPLACE: a database that ran this chain under its earlier versions runs it
-- again, in the same order, and ends where a fresh database does.

CREATE OR REPLACE FUNCTION audit_privacy_reason_codes() RETURNS text[] LANGUAGE sql IMMUTABLE
AS $$SELECT ARRAY['retention','test','dsr_request','restriction_expired','the website lead could not be sent its privacy notice','subject_data_removed','legal hold ended: the postponed content erasure was completed','retention: the record''s content was erased','controller release: the record''s content was erased','noise disposition: the content of the message was erased']::text[]$$;

CREATE OR REPLACE FUNCTION audit_redacted_image(kind text, payload jsonb) RETURNS jsonb
LANGUAGE plpgsql IMMUTABLE AS $$DECLARE
  kept jsonb := '{}'::jsonb;
  field text;
  held jsonb;
  value text;
BEGIN
  IF jsonb_typeof(payload) IS DISTINCT FROM 'object' THEN
    RETURN NULL;
  END IF;
  FOR field, held IN SELECT key, item FROM jsonb_each(payload) entry(key, item) LOOP
    value := held #>> '{}';
    IF field = 'status' AND jsonb_typeof(held) = 'string'
       AND ((kind = 'lead' AND value IN ('new','contacted','engaged','promoted','disqualified'))
         OR (kind = 'data_subject_request' AND value IN ('open','in_progress','fulfilled','rejected'))) THEN
      kept := kept || jsonb_build_object(field, held);
    ELSIF kind = 'data_subject_request' AND field IN ('resolution','dispute_open')
          AND jsonb_typeof(held) = 'boolean' THEN
      kept := kept || jsonb_build_object(field, held);
    ELSIF kind = 'activity' THEN
      IF (field = 'meeting_status' AND value IN ('booked','held','no_show','canceled'))
         OR (field = 'occurred_precision' AND value IN ('instant','day'))
         OR (field = 'occurred_at' AND jsonb_typeof(held) = 'string'
             AND value ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+(Z|[+-][0-9]{2}:[0-9]{2})$') THEN
        kept := kept || jsonb_build_object(field, held);
      ELSIF field = 'commitment_admission_redacted' AND held <> 'null'::jsonb THEN
        kept := kept || jsonb_build_object(field, true);
      END IF;
    END IF;
  END LOOP;
  RETURN nullif(kept, '{}'::jsonb);
END;
$$;

CREATE OR REPLACE FUNCTION audit_redacted_evidence(certificate boolean, payload jsonb, verb text DEFAULT '') RETURNS jsonb
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
    ELSIF field IN ('cause','class','basis','retention_action','purge_reason','policy','request_id','restricted_until')
          AND jsonb_typeof(held) = 'string' THEN
      kept := kept || jsonb_build_object(field, held);
    ELSIF field IN ('reason','decided_by_name') AND verb IN ('release','pin','restrict') THEN
      kept := kept || jsonb_build_object(field, held);
    ELSIF field = 'reason' AND ((held #>> '{}') = ANY(audit_privacy_reason_codes())
      OR ((held #>> '{}') ~ '^dsr:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
          AND (held #>> '{}') <> 'dsr:00000000-0000-0000-0000-000000000000')) THEN
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
     AND NEW.before IS NOT DISTINCT FROM audit_redacted_image(OLD.entity_type, OLD.before)
     AND NEW.after IS NOT DISTINCT FROM audit_redacted_image(OLD.entity_type, OLD.after)
     AND NEW.evidence = audit_redacted_evidence(
       OLD.action IN ('erase','anonymize','expire','release','pin','restrict') AND OLD.before IS NULL AND OLD.after IS NULL, OLD.evidence, OLD.action)
     AND (to_jsonb(NEW) - ARRAY['before','after','evidence']) =
         (to_jsonb(OLD) - ARRAY['before','after','evidence'])
     AND EXISTS (
       SELECT 1 FROM audit_log certificate
       WHERE certificate.before IS NULL AND certificate.after IS NULL
         AND certificate.source_xid = pg_current_xact_id()
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
