-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The trigger goes back to the version that does not read given_by_admin
-- before the column is dropped, or every later write would fail.
SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION communication_instruction_refuse_rewrite() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'a communication instruction is the record of a decision and is never deleted'
      USING ERRCODE = 'check_violation',
            CONSTRAINT = 'communication_instruction_immutable';
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

ALTER TABLE contact_acquisition_evidence
    DROP COLUMN request_retracted_by,
    DROP COLUMN request_retracted_at;

ALTER TABLE consent_qualifying_event
    DROP COLUMN asks_for_quote,
    DROP COLUMN retracted_by,
    DROP COLUMN retracted_at;

ALTER TABLE communication_instruction
    DROP COLUMN given_by_admin;
