SET LOCAL lock_timeout = '3s';

-- A message can go out for two different reasons, and the record has to say
-- which.
--
-- Until now there was one: the engine allowed it. A decision row reading `deny`
-- described a message that did not go. That is what made the record readable —
-- `verdict` alone answered both "what did the engine think" and "did this send".
--
-- A controller-directed send breaks that pairing on purpose. A designated human
-- reads what the engine refused, takes responsibility in writing, and the exact
-- message goes anyway. The engine's answer does not change, and it must not:
-- rewriting the decision to `allow` would erase the only record that anybody
-- overrode anything, and the next reader would see a message that consent
-- permitted. It did not.
--
-- So the verdict stays `deny` and this column says the message went under a
-- named person's instruction rather than under the engine's permission.

-- On the DECISION, because the question "under what authority was this
-- recipient written to" is per recipient: a message to three people may be
-- allowed for two of them and directed for the third.
ALTER TABLE communication_decision
    ADD COLUMN execution_authority text NOT NULL DEFAULT 'supported',
    ADD COLUMN instruction_id uuid REFERENCES communication_instruction(id) ON DELETE RESTRICT;

-- 'supported' is the default and the backfill, and it is the truthful value for
-- every row written before this column existed: the only way a message could
-- have been sent was the engine allowing it.
ALTER TABLE communication_decision
    ADD CONSTRAINT communication_decision_execution_authority
        CHECK (execution_authority IN ('supported', 'instruction')),
    -- An instruction is named by exactly the rows that went out under one. A
    -- row claiming the authority without naming the instruction cannot be
    -- audited, and one naming an instruction while claiming the engine's
    -- permission attributes a decision to somebody who did not make it.
    ADD CONSTRAINT communication_decision_instruction_shape
        CHECK ((execution_authority = 'instruction') = (instruction_id IS NOT NULL));

-- ON DELETE RESTRICT, not CASCADE. The decision row is Art. 5(2)
-- accountability evidence and outlives everything; deleting the instruction it
-- names would leave a sent message whose authority nothing records. Refusing
-- the delete is the honest answer — an instruction that authorized a send is
-- part of that send's history.

-- On the DELIVERY too, so the worker that hands the message to the provider can
-- see the authority without reading the per-recipient decisions.
--
-- THE WORKER IS THE REASON THIS COLUMN EXISTS RATHER THAN BEING DERIVED. A
-- build that predates directed sends does not know what 'instruction' means,
-- and a delivery it cannot understand must park rather than go: an older worker
-- reading an unfamiliar authority would otherwise send a message it has no
-- rules for.
ALTER TABLE comms_outbound
    ADD COLUMN execution_authority text NOT NULL DEFAULT 'supported',
    ADD COLUMN instruction_id uuid REFERENCES communication_instruction(id) ON DELETE RESTRICT;

ALTER TABLE comms_outbound
    ADD CONSTRAINT comms_outbound_execution_authority
        CHECK (execution_authority IN ('supported', 'instruction')),
    ADD CONSTRAINT comms_outbound_instruction_shape
        CHECK ((execution_authority = 'instruction') = (instruction_id IS NOT NULL));

-- ONE DELIVERY PER INSTRUCTION. An instruction authorizes ONE message to ONE
-- envelope, and it is spent when that message goes. Without this, a bug or a
-- replay could hand the same written decision to a second delivery — the
-- recipient receives two messages, and the human who signed for one is recorded
-- as having signed for both.
CREATE UNIQUE INDEX comms_outbound_one_delivery_per_instruction
    ON comms_outbound (instruction_id)
    WHERE instruction_id IS NOT NULL;

-- THE MESSAGE THEY ACTUALLY READ, fingerprinted at the moment they decided.
--
-- A decision is about one message. Without this the only record of which
-- message would be the held row the review names — and that row can be edited
-- after the decision is made, so comparing the message against it would compare
-- it against itself and find every message unchanged.
--
-- The fingerprint is of the AUTHOR'S text, not of what goes on the wire: the
-- rendered body carries a signature and an unsubscribe footer whose withdrawal
-- link is minted per send, so no two renderings of one message are alike.
ALTER TABLE communication_instruction
    ADD COLUMN acknowledged_wording bytea;

-- The delivery an instruction was spent on, so a reader going the other way —
-- from the recorded decision to the message it produced — does not have to
-- search the deliveries for it.
ALTER TABLE communication_instruction
    ADD COLUMN delivery_id uuid;

-- A consumed instruction names the message it was spent on, and only a consumed
-- one may. The table already required a consumed row to say WHEN
-- (communication_instruction_consumption_shape); this says WHAT.
--
-- Without it, a directed row could carry a delivery id — claiming to have
-- authorized a message while still reading as an unspent decision, so a second
-- send could spend it again.
ALTER TABLE communication_instruction
    ADD CONSTRAINT communication_instruction_spent_on_shape
        CHECK ((status = 'consumed') = (delivery_id IS NOT NULL));

-- AND THE DELIVERY, ONCE NAMED, IS FROZEN LIKE THE REST OF THE DECISION.
--
-- The trigger beside the instruction table freezes everything that records what
-- was decided and lets through what later happened to it — status, consumption,
-- revocation. delivery_id is new and would fall on the permissive side by
-- default, which is right while it is null and wrong the moment it is set: a
-- consumed instruction re-pointed at another message would say a named human
-- signed for a message they never saw.
--
-- Null to non-null is the consumption itself and must pass. Anything after
-- that is a rewrite.
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
