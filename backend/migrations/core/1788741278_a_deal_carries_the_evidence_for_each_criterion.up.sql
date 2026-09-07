-- The evidence ledger: what was OBSERVED about a deal against one of its
-- stage's exit criteria.
--
-- The observation half of stage intelligence. stage_exit_criterion says what a
-- stage requires; a row here says a particular deal met one, cites where that
-- was seen, and records who authored the thing cited.
--
-- AUTHOR SIDE IS THE POINT. A criterion naming something the BUYER did is
-- never settled by a message our own side wrote — a rep asserting "they
-- confirmed the budget" is not the buyer confirming it. The engine computes
-- author_side from the activity's direction, its participants and the
-- installation's own domains; the column exists so a later reader can see the
-- judgement rather than re-derive it from records that may since have moved.
-- A model never writes this column.
--
-- COMMITMENT separates what was AGREED from what was merely PROPOSED. "We
-- could do a demo next week" and "yes, Thursday works" are different facts,
-- and only the second settles an event_held criterion.
--
-- Evidence is REFUTED, never deleted: a human marking a claim incorrect is
-- itself a fact worth keeping, and the row is what a later reader consults to
-- see why a stage move was reversed.
--
-- No workspace column and no policy: one installation holds one organization.
SET LOCAL lock_timeout = '5s';

CREATE TABLE deal_stage_evidence (
    id uuid DEFAULT uuidv7() NOT NULL,
    deal_id uuid NOT NULL,
    criterion_id uuid NOT NULL,
    source_type text NOT NULL,
    source_id uuid NOT NULL,
    -- 1-based line numbers into a transcript, when the claim quotes one.
    source_lines integer[],
    snippet text,
    author_side text NOT NULL,
    commitment text NOT NULL,
    met boolean NOT NULL,
    confidence numeric(3,2),
    observed_at timestamptz NOT NULL,
    extracted_by text NOT NULL,
    contradicted_by uuid,
    refuted_at timestamptz,
    refuted_by uuid,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT deal_stage_evidence_pkey PRIMARY KEY (id),
    CONSTRAINT deal_stage_evidence_source_type_check
      CHECK (source_type IN ('activity', 'contract')),
    CONSTRAINT deal_stage_evidence_author_side_check
      CHECK (author_side IN ('buyer', 'seller', 'unknown')),
    CONSTRAINT deal_stage_evidence_commitment_check
      CHECK (commitment IN ('agreed', 'proposed', 'none')),
    CONSTRAINT deal_stage_evidence_snippet_len
      CHECK (snippet IS NULL OR length(snippet) <= 500),
    CONSTRAINT deal_stage_evidence_confidence_range
      CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
    -- A refutation is a decision somebody made, so it has both a moment and an
    -- author or neither. One without the other is a refutation nobody owns.
    CONSTRAINT deal_stage_evidence_refutation_whole
      CHECK ((refuted_at IS NULL) = (refuted_by IS NULL)),
    -- Deterministic writers assert what a record already states, so they never
    -- carry a confidence; a model's claim always does. extracted_by is the
    -- writer's own name, and 'deterministic' is the one that means "no model
    -- was asked".
    CONSTRAINT deal_stage_evidence_deterministic_is_certain
      CHECK (extracted_by <> 'deterministic' OR confidence IS NULL),
    CONSTRAINT deal_stage_evidence_deal_fkey
      FOREIGN KEY (deal_id) REFERENCES deal(id) ON DELETE CASCADE,
    -- RESTRICT, not CASCADE: a criterion is archived rather than deleted
    -- precisely so the evidence citing it stays readable, and a delete that
    -- took the evidence with it would defeat that.
    CONSTRAINT deal_stage_evidence_criterion_fkey
      FOREIGN KEY (criterion_id) REFERENCES stage_exit_criterion(id) ON DELETE RESTRICT,
    CONSTRAINT deal_stage_evidence_contradicted_fkey
      FOREIGN KEY (contradicted_by) REFERENCES deal_stage_evidence(id) ON DELETE SET NULL,
    CONSTRAINT deal_stage_evidence_refuted_by_fkey
      FOREIGN KEY (refuted_by) REFERENCES app_user(id) ON DELETE SET NULL
);

-- One deterministic claim per (deal, criterion, source). A contract turning
-- active twice, or a redelivered event, must not write the same fact again —
-- the bus is at-least-once and the trigger runs per delivery.
CREATE UNIQUE INDEX deal_stage_evidence_one_per_source_ux
  ON deal_stage_evidence USING btree (deal_id, criterion_id, source_type, source_id);

CREATE INDEX deal_stage_evidence_deal_ix
  ON deal_stage_evidence USING btree (deal_id, criterion_id)
  WHERE refuted_at IS NULL;

COMMENT ON COLUMN deal_stage_evidence.author_side IS
  'Who authored the cited thing, computed by the engine from direction, participants and the installation''s own domains. A model never writes this: a buyer milestone settled by seller-authored text is the defect this column exists to make visible.';
COMMENT ON COLUMN deal_stage_evidence.commitment IS
  'Whether the cited text AGREED to the thing or merely proposed it. "We could meet Thursday" is proposed; "Thursday works" is agreed.';
COMMENT ON COLUMN deal_stage_evidence.extracted_by IS
  'Which writer made this claim — ''deterministic'' for a record-derived one, otherwise the model task that read it.';
COMMENT ON COLUMN deal_stage_evidence.refuted_at IS
  'When a human marked this claim incorrect. The row stays: why a stage move was reversed is a question asked later.';

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE deal_stage_evidence TO margince_app;

CREATE TRIGGER trg_deal_stage_evidence_updated
  BEFORE UPDATE ON deal_stage_evidence
  FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();

-- A stage move made through an approval, and the move that undid one.
-- Additive: every existing row keeps both as NULL, which reads as "a human
-- moved this deal and nothing has reversed it".
ALTER TABLE deal_stage_history
  ADD COLUMN approval_id uuid,
  ADD COLUMN reversal_of uuid;

ALTER TABLE deal_stage_history
  ADD CONSTRAINT deal_stage_history_reversal_fkey
  FOREIGN KEY (reversal_of) REFERENCES deal_stage_history(id) ON DELETE SET NULL;

COMMENT ON COLUMN deal_stage_history.approval_id IS
  'The approval this move was applied under, when it was proposed rather than typed. NULL for a human''s own move.';
COMMENT ON COLUMN deal_stage_history.reversal_of IS
  'The move this one undid. Set by the revert path and by a human moving the deal back inside the undo window.';
