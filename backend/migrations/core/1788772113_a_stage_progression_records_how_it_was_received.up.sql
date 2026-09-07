-- How each proposed stage move was received, on its own ledger.
--
-- NOT the approval row, which is why this table exists. Approvals expire and
-- are cleaned up; the question "how often does this transition get accepted
-- unedited" is asked months later over a window, and an answer that decayed as
-- rows aged would report a rate measured on whatever happened to survive.
--
-- Rejecting a move and correcting its evidence are SEPARATE facts, and the
-- columns keep them apart. A rep who says "not yet" has not said the evidence
-- is wrong — they may agree with every claim and still want the deal where it
-- is — and a ledger that conflated the two would refute claims nobody disputed.
CREATE TABLE stage_progression_outcome (
    id uuid DEFAULT uuidv7() NOT NULL,
    approval_id uuid NOT NULL,
    deal_id uuid NOT NULL,
    pipeline_id uuid NOT NULL,
    from_stage_id uuid NOT NULL,
    to_stage_id uuid NOT NULL,
    -- The criterion kinds the move rested on, so the report can answer "which
    -- evidence do we actually accept" per transition rather than in aggregate.
    evidence_kinds text[] DEFAULT '{}'::text[] NOT NULL,
    outcome text NOT NULL,
    -- rejection_reason is the rep's own words, kept because a transition
    -- rejected forty times for one reason is a configuration problem and the
    -- reason is the only thing that says which.
    rejection_reason text,
    -- evidence_corrected records that a human separately marked a claim
    -- incorrect. Distinct from the outcome: a move can be approved with one
    -- claim corrected, and rejected with every claim standing.
    evidence_corrected boolean DEFAULT false NOT NULL,
    -- decided_by_system marks a move nobody was asked about. It separates the
    -- auto-applied rate from the accepted rate, and a report that could not
    -- tell them apart would read an autopilot's own output as evidence that
    -- people agree with it.
    decided_by_system boolean DEFAULT false NOT NULL,
    decided_at timestamptz DEFAULT now() NOT NULL,
    reversed_at timestamptz,
    reversed_by uuid,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT stage_progression_outcome_pkey PRIMARY KEY (id),
    CONSTRAINT stage_progression_outcome_kind CHECK (
        outcome IN ('proposed', 'approved_clean', 'approved_edited', 'rejected',
                    'auto_applied', 'reversed', 'expired', 'superseded')),
    -- A reversal names WHEN it happened, always. It does not require a
    -- surviving user, and requiring one would break the deletion it sits
    -- beside: reversed_by is ON DELETE SET NULL like every other app_user
    -- reference in this schema, so a paired-nullability check would turn the
    -- declared SET NULL into a constraint violation and refuse the delete.
    --
    -- The direction that IS enforced: nobody without a time. A person recorded
    -- against no instant answers a question nobody can place, whereas an
    -- instant whose person has since been erased is exactly what an
    -- anonymized workspace looks like — and the audit trail still holds who.
    CONSTRAINT stage_progression_outcome_reversal_is_timed
      CHECK (reversed_by IS NULL OR reversed_at IS NOT NULL),
    -- Only a rejection carries a reason. A reason on an approval would be a
    -- field the report counts and nobody wrote.
    CONSTRAINT stage_progression_outcome_reason_is_a_rejections
      CHECK (rejection_reason IS NULL OR outcome = 'rejected'),
    CONSTRAINT stage_progression_outcome_deal_fkey
      FOREIGN KEY (deal_id) REFERENCES deal(id) ON DELETE CASCADE,
    CONSTRAINT stage_progression_outcome_pipeline_fkey
      FOREIGN KEY (pipeline_id) REFERENCES pipeline(id) ON DELETE CASCADE,
    CONSTRAINT stage_progression_outcome_from_stage_fkey
      FOREIGN KEY (from_stage_id) REFERENCES stage(id) ON DELETE CASCADE,
    CONSTRAINT stage_progression_outcome_to_stage_fkey
      FOREIGN KEY (to_stage_id) REFERENCES stage(id) ON DELETE CASCADE,
    CONSTRAINT stage_progression_outcome_reversed_by_fkey
      FOREIGN KEY (reversed_by) REFERENCES app_user(id) ON DELETE SET NULL
);

-- One row per approval: a proposal is received once, and a second row for the
-- same decision would double-count it in every rate the report computes.
CREATE UNIQUE INDEX stage_progression_outcome_approval_ux
    ON stage_progression_outcome USING btree (approval_id);

-- The report's own access path: rates per transition over a window.
CREATE INDEX idx_stage_progression_outcome_transition
    ON stage_progression_outcome USING btree (pipeline_id, from_stage_id, to_stage_id, decided_at DESC);

-- The protection read: has this deal already said no to this target, and has
-- anything been learned since.
CREATE INDEX idx_stage_progression_outcome_deal_target
    ON stage_progression_outcome USING btree (deal_id, to_stage_id, decided_at DESC);

CREATE TRIGGER trg_stage_progression_outcome_updated
    BEFORE UPDATE ON stage_progression_outcome
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE stage_progression_outcome TO margince_app;
