-- Back to refusing every DELETE and UPDATE, with no exception.
CREATE OR REPLACE FUNCTION audit_log_immutable() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  RAISE EXCEPTION 'audit_log is append-only (attempted % on row %)', TG_OP, OLD.id
    USING ERRCODE = 'check_violation';
END; $$;

REVOKE UPDATE (before, after) ON TABLE audit_log FROM margince_app;
