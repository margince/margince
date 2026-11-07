SET LOCAL lock_timeout = '3s';

REVOKE UPDATE (before, after, evidence) ON audit_log FROM margince_app;

CREATE OR REPLACE FUNCTION audit_log_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'audit_log is append-only (attempted % on row %)', TG_OP, OLD.id
    USING ERRCODE = 'check_violation';
END;
$$;

DROP FUNCTION audit_redacted_evidence(boolean, jsonb, text);
DROP FUNCTION audit_privacy_reason_codes();
