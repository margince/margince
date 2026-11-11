SET LOCAL lock_timeout = '3s';
-- audit_log keeps the privacy redaction it already admits and gains one more
-- UPDATE: the repair of credentials stored before the masking rules. A DELETE
-- is still refused.
--
-- The repair's UPDATE needs all of these, in the transaction that writes it:
-- its transaction-local setting; a system audit row of the same record, written
-- by that transaction, saying the repair masked its images; a record type the
-- masking rules reach; and an image that keeps its shape, every changed string
-- now the old string with spans replaced by the mask marker. A mail under a
-- statutory hold keeps its trail. The masking rules themselves stay in Go.
--
-- A length bound would be the wrong test: the marker is longer than a short
-- secret, so it would refuse the rewrite of `Passwort: abc`.
--
-- The column grant this needs, UPDATE (before, after), is already held through
-- the privacy redaction's GRANT UPDATE (before, after, evidence).
-- audit_text_only_masked answers whether rewritten is was with one or more
-- spans replaced by the mask marker: the pieces between the markers must be
-- found in was, in order, the first as its prefix and the last as its suffix.
CREATE OR REPLACE FUNCTION audit_text_only_masked(was text, rewritten text) RETURNS boolean
LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE
  pieces text[] := string_to_array(rewritten, '[masked]');
  piece text;
  piece_count integer := array_length(pieces, 1);
  next_at integer;
  match_at integer;
BEGIN
  IF piece_count IS NULL OR piece_count < 2 OR left(was, length(pieces[1])) <> pieces[1] THEN
    RETURN false;
  END IF;
  next_at := length(pieces[1]) + 1;
  FOR item IN 2 .. piece_count - 1 LOOP
    piece := pieces[item];
    match_at := strpos(substr(was, next_at), piece);
    IF match_at = 0 THEN
      RETURN false;
    END IF;
    next_at := next_at + match_at - 1 + length(piece);
  END LOOP;
  RETURN length(was) - length(pieces[piece_count]) + 1 >= next_at
     AND right(was, length(pieces[piece_count])) = pieces[piece_count];
END;
$$;

CREATE OR REPLACE FUNCTION audit_image_only_masked(was jsonb, rewritten jsonb) RETURNS boolean
LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE
  field text;
  item integer;
BEGIN
  IF was IS NULL OR rewritten IS NULL THEN
    RETURN was IS NOT DISTINCT FROM rewritten;
  END IF;
  IF jsonb_typeof(was) <> jsonb_typeof(rewritten) THEN
    RETURN false;
  END IF;
  CASE jsonb_typeof(was)
  WHEN 'object' THEN
    IF ARRAY(SELECT jsonb_object_keys(was) ORDER BY 1)
       IS DISTINCT FROM ARRAY(SELECT jsonb_object_keys(rewritten) ORDER BY 1) THEN
      RETURN false;
    END IF;
    FOR field IN SELECT jsonb_object_keys(was) LOOP
      IF NOT audit_image_only_masked(was -> field, rewritten -> field) THEN
        RETURN false;
      END IF;
    END LOOP;
    RETURN true;
  WHEN 'array' THEN
    IF jsonb_array_length(was) <> jsonb_array_length(rewritten) THEN
      RETURN false;
    END IF;
    FOR item IN 0 .. jsonb_array_length(was) - 1 LOOP
      IF NOT audit_image_only_masked(was -> item, rewritten -> item) THEN
        RETURN false;
      END IF;
    END LOOP;
    RETURN true;
  WHEN 'string' THEN
    RETURN was = rewritten OR audit_text_only_masked(was #>> '{}', rewritten #>> '{}');
  ELSE
    RETURN was = rewritten;
  END CASE;
END;
$$;

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
  ELSIF TG_OP = 'UPDATE'
     AND current_setting('margince.audit_credential_mask', true) = 'on'
     AND OLD.entity_type IN ('activity', 'mail_draft', 'scheduled_send')
     AND NOT EXISTS (SELECT 1 FROM activity held
                      WHERE OLD.entity_type = 'activity' AND held.id = OLD.entity_id
                        AND held.restricted_at IS NOT NULL)
     AND (to_jsonb(NEW) - ARRAY['before','after']) = (to_jsonb(OLD) - ARRAY['before','after'])
     AND audit_image_only_masked(OLD.before, NEW.before)
     AND audit_image_only_masked(OLD.after, NEW.after)
     AND EXISTS (
       SELECT 1 FROM audit_log certificate
       WHERE certificate.source_xid = pg_current_xact_id()
         AND certificate.entity_type = OLD.entity_type AND certificate.entity_id = OLD.entity_id
         AND certificate.action = 'update' AND certificate.actor_type = 'system'
         AND certificate.before IS NULL AND certificate.after IS NULL
         AND certificate.evidence -> 'audit_credentials_masked' = 'true'::jsonb) THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'audit metadata is immutable; payload redaction needs a current privacy certificate'
    USING ERRCODE = 'check_violation';
END;
$$;
