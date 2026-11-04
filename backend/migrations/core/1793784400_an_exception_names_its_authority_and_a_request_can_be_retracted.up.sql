-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Three additions to what a send decision rests on.
--
-- communication_instruction.given_by_admin records whether an admin gave the
-- one-message exception. Staging and dispatch re-check the refusals before the
-- message goes, and a refusal only an admin may answer is answered only by an
-- exception an admin gave. Existing rows read false, which answers fewer
-- refusals, never more. A constant default adds the column without a rewrite.
--
-- consent_qualifying_event.retracted_at and retracted_by let the colleague who
-- recorded a request take it back. asks_for_quote says the request was for a
-- quote, the one kind of request a quote may rest on. Existing rows read false.
--
-- contact_acquisition_evidence.request_retracted_at and request_retracted_by
-- take back the request an acquisition row records. The row stays as the
-- record of how the contact was obtained; only sends stop resting on it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_instruction
    ADD COLUMN given_by_admin boolean NOT NULL DEFAULT false;

ALTER TABLE consent_qualifying_event
    ADD COLUMN retracted_at timestamptz,
    ADD COLUMN retracted_by text,
    ADD COLUMN asks_for_quote boolean NOT NULL DEFAULT false;

ALTER TABLE contact_acquisition_evidence
    ADD COLUMN request_retracted_at timestamptz,
    ADD COLUMN request_retracted_by text;

-- The instruction trigger freezes what was decided. Who gave the exception is
-- part of that, so it joins the frozen columns.
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
     AND NEW.reason_code IS NOT DISTINCT FROM OLD.reason_code
     AND NEW.given_by_admin IS NOT DISTINCT FROM OLD.given_by_admin THEN
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
     OR NEW.directed_at IS DISTINCT FROM OLD.directed_at
     OR NEW.given_by_admin IS DISTINCT FROM OLD.given_by_admin THEN
    RAISE EXCEPTION 'a communication instruction records what was decided and cannot be rewritten; only its status, consumption and revocation move'
      USING ERRCODE = 'check_violation',
            CONSTRAINT = 'communication_instruction_immutable';
  END IF;
  RETURN NEW;
END $$;
