SET LOCAL lock_timeout = '3s';

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

DROP FUNCTION audit_redaction_actions();
