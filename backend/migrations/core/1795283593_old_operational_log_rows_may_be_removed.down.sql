-- Every delete is refused again, and the index by actor comes back.
SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION system_log_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'UPDATE' AND OLD.action = 'login'
     AND OLD.detail ? 'email_hash' AND NOT OLD.detail ? 'email_key_id'
     AND NEW.detail->>'email_hash' ~ '^[0-9a-f]{64}$'
     AND NEW.detail->>'email_key_id' ~ '^[0-9a-f]{16}$'
     AND (NEW.detail - ARRAY['email_hash','email_key_id']) = (OLD.detail - 'email_hash')
     AND (to_jsonb(NEW) - 'detail') = (to_jsonb(OLD) - 'detail') THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'system_log is append-only (attempted % on row %)', TG_OP, OLD.id
    USING ERRCODE = 'check_violation';
END;
$$;

CREATE INDEX IF NOT EXISTS idx_system_log_actor ON system_log (actor_id, occurred_at DESC);
