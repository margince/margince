SET LOCAL lock_timeout = '3s';

-- Which key derived from the root key hashed a stored identifier. NULL marks
-- the unkeyed sha256 an install wrote before it had one; the boot check
-- refuses a process whose key did not make the keyed rows.
ALTER TABLE erasure_suppression ADD COLUMN key_id text;
ALTER TABLE communication_suppression ADD COLUMN address_key_id text;
ALTER TABLE contact ADD COLUMN anonymized_address_key_id text;

-- A preference link is no longer stored. An old link is found by its hash,
-- and a new one names its row and carries a MAC instead.
ALTER TABLE preference_token ADD COLUMN token_hash text;
ALTER TABLE preference_token ALTER COLUMN token DROP NOT NULL;

-- When a delivery's rendered body was cleared. The row stays for the
-- delivery log and the (subscription, event) dedupe.
ALTER TABLE webhook_delivery ADD COLUMN payload_pruned_at timestamptz;

-- The log stays append-only, with one exception: a failed-login row's
-- unkeyed address hash may be replaced once by its keyed form, a 64-digit
-- hex HMAC stamped with the 16-digit hex id of its key.
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
