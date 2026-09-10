SET LOCAL lock_timeout = '3s';

-- A named human deciding one refused message goes out anyway.
--
-- The engine refuses and records why. Sometimes the installation has a reason
-- the engine cannot see — a contract clause, a legal obligation, a subject who
-- asked in a room nobody logged — and somebody with the authority decides to
-- send.
--
-- THIS IS NOT A CONSENT GRANT AND MUST NEVER BE RECORDED AS ONE. The refusal
-- stays exactly where it is; this row sits beside it saying a person overrode
-- it, who they were, and what they said their reason was. A subject asking
-- later why they received a message must be able to be shown the refusal AND
-- the decision, not a grant nobody made.
CREATE TABLE communication_instruction (
    id uuid PRIMARY KEY DEFAULT uuidv7(),

    -- The review this answers. UNIQUE, because one refusal is directed once:
    -- two instructions against one review would be two people each believing
    -- they decided it, and no way to say which send went out under which.
    review_id uuid NOT NULL UNIQUE REFERENCES communication_review(id) ON DELETE CASCADE,

    -- WHO DECIDED. Not who sent — the two can differ once a reviewer directs
    -- somebody else's message, and the record must name the person whose
    -- authority it was.
    directed_by uuid NOT NULL REFERENCES app_user(id) ON DELETE RESTRICT,

    -- What they said, in their own words, and under which of the closed reasons.
    -- The reason is a vocabulary so a queue can be read; the explanation is what
    -- a dispute actually reads.
    reason_code text NOT NULL,
    explanation text NOT NULL,

    -- WHAT THEY ACKNOWLEDGED, and which version of it. The warning shown to a
    -- person directing a send is a compliance text that changes; a record
    -- naming no version cannot say what they were told.
    warning_version text NOT NULL,
    acknowledged_at timestamptz NOT NULL,

    -- THE FACTS THIS DECISION WAS MADE ON. An instruction directed against
    -- Tuesday's refusals must not authorize a send after Wednesday's objection:
    -- the send path compares this against the review's own state and refuses a
    -- decision that has gone stale.
    facts_as_of timestamptz NOT NULL,
    valid_until timestamptz NOT NULL,

    directed_at timestamptz NOT NULL DEFAULT now(),

    -- Where it got to. `directed` is live; `consumed` means a send went out
    -- under it; `revoked` means the decision was taken back before it was used;
    -- `expired` means nobody used it in time.
    status text NOT NULL DEFAULT 'directed',
    consumed_at timestamptz,
    revoked_at timestamptz,
    revoked_by uuid REFERENCES app_user(id) ON DELETE SET NULL,
    revoked_reason text,

    CONSTRAINT communication_instruction_reason CHECK (reason_code = ANY (ARRAY[
        'customer_requested_outside_crm'::text, 'contractual_necessity'::text,
        'legal_obligation'::text, 'other'::text])),
    CONSTRAINT communication_instruction_status CHECK (status = ANY (ARRAY[
        'directed'::text, 'consumed'::text, 'revoked'::text, 'expired'::text])),
    -- A consumed instruction says when. A live one must not claim a moment.
    CONSTRAINT communication_instruction_consumption_shape CHECK (
        (status = 'consumed'::text) = (consumed_at IS NOT NULL)),
    -- A revocation names who took it back and why. "Revoked" with nobody behind
    -- it is a decision reversed by nobody, which is not a thing that happened.
    CONSTRAINT communication_instruction_revocation_shape CHECK (
        (status = 'revoked'::text) =
        (revoked_at IS NOT NULL AND revoked_by IS NOT NULL AND revoked_reason IS NOT NULL)),
    -- An explanation is required and must say something. A blank one is an
    -- acknowledgement nobody can be held to.
    CONSTRAINT communication_instruction_explained CHECK (
        length(btrim(explanation)) > 0),
    -- The window closes after the facts it was taken on, never before.
    CONSTRAINT communication_instruction_window CHECK (valid_until > facts_as_of)
);

CREATE INDEX communication_instruction_live
    ON communication_instruction (valid_until)
    WHERE status = 'directed';

CREATE INDEX communication_instruction_by_director
    ON communication_instruction (directed_by, directed_at DESC);

-- AN INSTRUCTION IS NOT REWRITABLE.
--
-- It is the record of what one person decided and what they were told when they
-- decided it. Every field above except the four that record what LATER happened
-- to it is frozen: a reason edited after the fact, or a warning version quietly
-- corrected, would let the account of an override be improved by whoever gave
-- it — which is the one thing a dispute about that override needs not to be
-- possible.
CREATE FUNCTION communication_instruction_refuse_rewrite() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'a communication instruction is the record of a decision and is never deleted'
      USING ERRCODE = 'check_violation',
            CONSTRAINT = 'communication_instruction_immutable';
  END IF;
  IF NEW.review_id IS DISTINCT FROM OLD.review_id
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

CREATE TRIGGER communication_instruction_refuse_rewrite
    BEFORE DELETE OR UPDATE ON communication_instruction
    FOR EACH ROW EXECUTE FUNCTION communication_instruction_refuse_rewrite();
