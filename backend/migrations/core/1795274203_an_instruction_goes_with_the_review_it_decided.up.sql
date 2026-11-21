-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A workspace reset clears communication_review, and its CASCADE reaches the
-- instruction that decided it. The guard refused every delete, so the reset
-- stopped on the first directed send. It now refuses a direct delete only.
-- The audit_log row of the decision is what a reset keeps.
CREATE OR REPLACE FUNCTION communication_instruction_refuse_rewrite() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  -- A row goes only with the review it decided, through the CASCADE. A direct
  -- delete is refused. The CASCADE has already removed the review by the time
  -- this fires, so the review is gone when the delete is its own.
  IF TG_OP = 'DELETE' THEN
    IF EXISTS (SELECT 1 FROM communication_review r WHERE r.id = OLD.review_id) THEN
      RAISE EXCEPTION 'a communication instruction is the record of a decision and is removed only with the review it decided'
        USING ERRCODE = 'check_violation',
              CONSTRAINT = 'communication_instruction_immutable';
    END IF;
    RETURN OLD;
  END IF;
  IF NEW.acknowledged_wording IS DISTINCT FROM OLD.acknowledged_wording THEN
    RAISE EXCEPTION 'a communication instruction records the message that was acknowledged and cannot be re-pointed at another'
      USING ERRCODE = 'check_violation',
            CONSTRAINT = 'communication_instruction_immutable';
  END IF;
  IF OLD.delivery_id IS NOT NULL AND NEW.delivery_id IS DISTINCT FROM OLD.delivery_id THEN
    RAISE EXCEPTION 'a communication instruction is spent on one message and cannot be re-pointed at another'
      USING ERRCODE = 'check_violation',
            CONSTRAINT = 'communication_instruction_immutable';
  END IF;
  IF NEW.explanation = '[erased]'
     AND NEW.id IS NOT DISTINCT FROM OLD.id
     AND NEW.review_id IS NOT DISTINCT FROM OLD.review_id
     AND NEW.directed_by IS NOT DISTINCT FROM OLD.directed_by
     AND NEW.directed_by_agent IS NOT DISTINCT FROM OLD.directed_by_agent
     AND NEW.reason_code IS NOT DISTINCT FROM OLD.reason_code THEN
    RETURN NEW;
  END IF;
  IF NEW.id IS DISTINCT FROM OLD.id
     OR NEW.review_id IS DISTINCT FROM OLD.review_id
     OR NEW.directed_by IS DISTINCT FROM OLD.directed_by
     OR NEW.directed_by_agent IS DISTINCT FROM OLD.directed_by_agent
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
