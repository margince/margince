SET LOCAL lock_timeout = '3s';

-- THE TRIGGER FIRST, and restored to what it was.
--
-- The up migration replaced the function to freeze two new columns. Dropping
-- those columns without putting the original function back would leave a
-- trigger referencing fields that no longer exist, and every later write to
-- this table would fail — a rollback that breaks the thing it was rolling back.
CREATE OR REPLACE FUNCTION communication_instruction_refuse_rewrite() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'a communication instruction is the record of a decision and is never deleted'
      USING ERRCODE = 'check_violation',
            CONSTRAINT = 'communication_instruction_immutable';
  END IF;
  IF NEW.explanation = '[erased]'
     AND NEW.id IS NOT DISTINCT FROM OLD.id
     AND NEW.review_id IS NOT DISTINCT FROM OLD.review_id
     AND NEW.directed_by IS NOT DISTINCT FROM OLD.directed_by
     AND NEW.reason_code IS NOT DISTINCT FROM OLD.reason_code THEN
    RETURN NEW;
  END IF;
  IF NEW.id IS DISTINCT FROM OLD.id
     OR NEW.review_id IS DISTINCT FROM OLD.review_id
     OR NEW.directed_by IS DISTINCT FROM OLD.directed_by
     OR NEW.reason_code IS DISTINCT FROM OLD.reason_code
     OR NEW.explanation IS DISTINCT FROM OLD.explanation
     OR NEW.warning_version IS DISTINCT FROM OLD.warning_version
     OR NEW.acknowledged_at IS DISTINCT FROM OLD.acknowledged_at
     OR NEW.facts_as_of IS DISTINCT FROM OLD.facts_as_of
     OR NEW.valid_until IS DISTINCT FROM OLD.valid_until
     OR NEW.directed_at IS DISTINCT FROM OLD.directed_at THEN
    RAISE EXCEPTION 'a communication instruction records what was decided and cannot be rewritten; only its status, consumption and revocation move'
      USING ERRCODE = 'check_violation',
            CONSTRAINT = 'communication_instruction_immutable';
  END IF;
  RETURN NEW;
END $$;

ALTER TABLE communication_instruction
    DROP CONSTRAINT IF EXISTS communication_instruction_spent_on_shape,
    DROP COLUMN IF EXISTS delivery_id,
    DROP COLUMN IF EXISTS acknowledged_wording;

DROP INDEX IF EXISTS comms_outbound_one_delivery_per_instruction;

ALTER TABLE comms_outbound
    DROP CONSTRAINT IF EXISTS comms_outbound_instruction_shape,
    DROP CONSTRAINT IF EXISTS comms_outbound_execution_authority,
    DROP COLUMN IF EXISTS instruction_id,
    DROP COLUMN IF EXISTS execution_authority;

ALTER TABLE communication_decision
    DROP CONSTRAINT IF EXISTS communication_decision_instruction_shape,
    DROP CONSTRAINT IF EXISTS communication_decision_execution_authority,
    DROP COLUMN IF EXISTS instruction_id,
    DROP COLUMN IF EXISTS execution_authority;
