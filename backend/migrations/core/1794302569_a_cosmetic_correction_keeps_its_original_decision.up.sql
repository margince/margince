-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_instruction
  ADD COLUMN acknowledged_scope varchar(64),
  ADD COLUMN corrected_wording bytea;

CREATE FUNCTION guard_instruction_wording_correction() RETURNS trigger
LANGUAGE plpgsql AS $$BEGIN
  IF NEW.acknowledged_scope IS NOT NULL AND NEW.acknowledged_scope !~ '^[a-f0-9]{64}$' THEN
    RAISE EXCEPTION 'an instruction scope must be a complete fingerprint' USING ERRCODE='check_violation';
  END IF;
  IF NEW.corrected_wording IS NOT NULL AND octet_length(NEW.corrected_wording) <> 32 THEN
    RAISE EXCEPTION 'a wording correction must be a complete fingerprint' USING ERRCODE='check_violation';
  END IF;
  IF TG_OP = 'INSERT' THEN
    IF NEW.corrected_wording IS NOT NULL THEN
      RAISE EXCEPTION 'a correction needs an existing decision' USING ERRCODE='check_violation';
    END IF;
  ELSE
    IF NEW.acknowledged_scope IS DISTINCT FROM OLD.acknowledged_scope THEN
      RAISE EXCEPTION 'an instruction scope is immutable' USING ERRCODE='check_violation';
    END IF;
    IF NEW.corrected_wording IS DISTINCT FROM OLD.corrected_wording AND
       (OLD.status <> 'directed' OR NEW.status <> 'directed' OR OLD.delivery_id IS NOT NULL
        OR OLD.acknowledged_scope IS NULL OR OLD.acknowledged_wording IS NULL
        OR NEW.corrected_wording IS NULL) THEN
      RAISE EXCEPTION 'only an unspent decision with a frozen message can retain a correction'
        USING ERRCODE='check_violation';
    END IF;
  END IF;
  RETURN NEW;
END;$$;

CREATE TRIGGER communication_instruction_wording_correction
  BEFORE INSERT OR UPDATE ON communication_instruction
  FOR EACH ROW EXECUTE FUNCTION guard_instruction_wording_correction();
