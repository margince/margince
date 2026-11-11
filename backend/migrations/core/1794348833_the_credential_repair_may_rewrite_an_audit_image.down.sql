SET LOCAL lock_timeout = '3s';
-- Back to the privacy redaction alone, as the migration before this one left it.
CREATE OR REPLACE FUNCTION audit_log_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'UPDATE'
     AND NEW.before IS NOT DISTINCT FROM audit_redacted_image(OLD.entity_type, OLD.before)
     AND NEW.after IS NOT DISTINCT FROM audit_redacted_image(OLD.entity_type, OLD.after)
     AND NEW.evidence = audit_evidence_after_erasure(OLD.action,OLD.before,OLD.after,OLD.evidence)
     AND (to_jsonb(NEW) - ARRAY['before','after','evidence']) =
         (to_jsonb(OLD) - ARRAY['before','after','evidence'])
     AND EXISTS (
       SELECT 1 FROM audit_log certificate
       WHERE certificate.before IS NULL AND certificate.after IS NULL
         AND certificate.source_xid = pg_current_xact_id()
         AND ((certificate.entity_type = OLD.entity_type
               AND certificate.entity_id = OLD.entity_id
               AND certificate.action=ANY(audit_redaction_actions()))
           OR (certificate.action = 'update' AND certificate.entity_type = 'audit_log'
               AND certificate.entity_id = OLD.id
               AND EXISTS(SELECT 1 FROM audit_log boundary
                WHERE boundary.id=audit_redaction_boundary(certificate.evidence)
                AND certificate.evidence=jsonb_build_object('payload_redaction',true,'legacy_erased_record',true,'boundary',boundary.id::text)
                AND boundary.action=ANY(audit_redaction_actions()) AND boundary.before IS NULL AND boundary.after IS NULL
                AND (boundary.evidence ? 'cause' OR boundary.evidence ? 'retention_action' OR boundary.evidence ? 'emails_suppressed')
                AND (OLD.occurred_at,OLD.id)<=(boundary.occurred_at,boundary.id)))
           OR (certificate.action = 'update' AND certificate.entity_type = 'audit_log'
               AND certificate.evidence = '{"payload_redaction":true}'::jsonb
               AND EXISTS (
                 SELECT 1 FROM audit_log boundary
                 WHERE boundary.id = certificate.entity_id
                   AND boundary.entity_type = OLD.entity_type AND boundary.entity_id = OLD.entity_id
                   AND boundary.action=ANY(audit_redaction_actions())
                   AND boundary.before IS NULL AND boundary.after IS NULL
                   AND (OLD.occurred_at, OLD.id) <= (boundary.occurred_at, boundary.id))))
     ) THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'audit metadata is immutable; payload redaction needs a current privacy certificate'
    USING ERRCODE = 'check_violation';
END;
$$;

DROP FUNCTION audit_image_only_masked(jsonb, jsonb);
DROP FUNCTION audit_text_only_masked(text, text);
