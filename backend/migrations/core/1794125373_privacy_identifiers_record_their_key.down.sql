SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION system_log_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'system_log is append-only (attempted % on row %)', TG_OP, OLD.id
    USING ERRCODE = 'check_violation';
END;
$$;

ALTER TABLE webhook_delivery DROP COLUMN payload_pruned_at;
ALTER TABLE preference_token ALTER COLUMN token SET NOT NULL;
ALTER TABLE preference_token DROP COLUMN token_hash;
ALTER TABLE contact DROP COLUMN anonymized_address_key_id;
ALTER TABLE communication_suppression DROP COLUMN address_key_id;
ALTER TABLE erasure_suppression DROP COLUMN key_id;
