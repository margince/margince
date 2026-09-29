-- Deal Scout's suggestions: a company with no open deal where the evidence says
-- commercial motion is under way, put in front of a rep to accept or dismiss.
--
-- The suggestion carries typed columns rather than a payload because the
-- pipeline board reads it by stage. Its evidence is one row per item, so a
-- reader's right to see the suggestion can be asked item by item: a suggestion
-- is visible only to someone who may read EVERY piece of its evidence.
SET LOCAL lock_timeout = '5s';

CREATE TABLE deal_suggestion (
    id uuid DEFAULT uuidv7() NOT NULL,
    -- open_deal is the only kind written today; the other two are reserved so
    -- the next kinds arrive without a migration.
    kind text DEFAULT 'open_deal' NOT NULL,
    state text DEFAULT 'open' NOT NULL,
    company_id uuid NOT NULL,
    pipeline_id uuid NOT NULL,
    proposed_stage_id uuid NOT NULL,
    -- What kind of evidence leads, as a code a reader's client words. The
    -- proposed deal's name is the company's name and this hint, so nothing
    -- here is text copied out of a message.
    name_hint text NOT NULL,
    proposed_amount_minor bigint,
    currency text,
    proposed_close_date date,
    confidence numeric(4,3) NOT NULL,
    -- The company and the evidence ids, hashed: the same evidence never
    -- raises a second suggestion.
    fingerprint text NOT NULL,
    -- How many evidence rows the suggestion was written with. The visibility
    -- rule compares it with the rows still present, so losing one to an
    -- erasure hides the suggestion rather than showing it to more readers.
    evidence_count integer NOT NULL,
    -- The newest evidence time. A dismissal re-arms only for evidence newer
    -- than the decision.
    evidence_through timestamptz NOT NULL,
    accepted_deal_id uuid,
    decided_by uuid,
    decided_at timestamptz,
    captured_by text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT deal_suggestion_pkey PRIMARY KEY (id),
    CONSTRAINT uq_deal_suggestion_fingerprint UNIQUE (fingerprint),
    CONSTRAINT deal_suggestion_kind_check CHECK (kind IN ('open_deal', 'advance_stage', 'revive')),
    CONSTRAINT deal_suggestion_state_check CHECK (state IN ('open', 'accepted', 'dismissed', 'superseded')),
    CONSTRAINT deal_suggestion_decided_shape CHECK ((state = 'open') = (decided_at IS NULL)),
    CONSTRAINT deal_suggestion_accepted_names_deal CHECK (accepted_deal_id IS NULL OR state = 'accepted'),
    CONSTRAINT deal_suggestion_money_pair CHECK ((proposed_amount_minor IS NULL) = (currency IS NULL)),
    CONSTRAINT deal_suggestion_amount_nonnegative CHECK (proposed_amount_minor IS NULL OR proposed_amount_minor >= 0),
    CONSTRAINT deal_suggestion_confidence_check CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT deal_suggestion_evidence_count_check CHECK (evidence_count > 0),
    CONSTRAINT deal_suggestion_name_hint_check CHECK (name_hint IN ('proposal_sent', 'opportunity_signalled', 'meeting_held'))
);

CREATE TABLE deal_suggestion_evidence (
    id uuid DEFAULT uuidv7() NOT NULL,
    suggestion_id uuid NOT NULL,
    kind text NOT NULL,
    activity_id uuid,
    signal_id uuid,
    attachment_id uuid,
    occurred_at timestamptz NOT NULL,

    CONSTRAINT deal_suggestion_evidence_pkey PRIMARY KEY (id),
    CONSTRAINT deal_suggestion_evidence_kind_check CHECK (kind IN ('meeting', 'signal', 'attachment')),
    CONSTRAINT deal_suggestion_evidence_shape CHECK (
        (kind = 'meeting' AND activity_id IS NOT NULL AND signal_id IS NULL AND attachment_id IS NULL)
     OR (kind = 'signal' AND signal_id IS NOT NULL AND activity_id IS NULL AND attachment_id IS NULL)
     OR (kind = 'attachment' AND attachment_id IS NOT NULL AND activity_id IS NULL AND signal_id IS NULL))
);

ALTER TABLE deal_suggestion
    ADD CONSTRAINT deal_suggestion_company_id_fkey FOREIGN KEY (company_id)
    REFERENCES company(id) ON DELETE CASCADE;
ALTER TABLE deal_suggestion
    ADD CONSTRAINT deal_suggestion_pipeline_id_fkey FOREIGN KEY (pipeline_id)
    REFERENCES pipeline(id) ON DELETE CASCADE;
ALTER TABLE deal_suggestion
    ADD CONSTRAINT deal_suggestion_proposed_stage_id_fkey FOREIGN KEY (proposed_stage_id)
    REFERENCES stage(id) ON DELETE CASCADE;
ALTER TABLE deal_suggestion
    ADD CONSTRAINT deal_suggestion_accepted_deal_id_fkey FOREIGN KEY (accepted_deal_id)
    REFERENCES deal(id) ON DELETE SET NULL;
ALTER TABLE deal_suggestion
    ADD CONSTRAINT deal_suggestion_decided_by_fkey FOREIGN KEY (decided_by)
    REFERENCES app_user(id) ON DELETE SET NULL;

-- An evidence row goes with the record it cites. The suggestion stays, and
-- evidence_count is what then hides it.
ALTER TABLE deal_suggestion_evidence
    ADD CONSTRAINT deal_suggestion_evidence_suggestion_id_fkey FOREIGN KEY (suggestion_id)
    REFERENCES deal_suggestion(id) ON DELETE CASCADE;
ALTER TABLE deal_suggestion_evidence
    ADD CONSTRAINT deal_suggestion_evidence_activity_id_fkey FOREIGN KEY (activity_id)
    REFERENCES activity(id) ON DELETE CASCADE;
ALTER TABLE deal_suggestion_evidence
    ADD CONSTRAINT deal_suggestion_evidence_signal_id_fkey FOREIGN KEY (signal_id)
    REFERENCES signal(id) ON DELETE CASCADE;
ALTER TABLE deal_suggestion_evidence
    ADD CONSTRAINT deal_suggestion_evidence_attachment_id_fkey FOREIGN KEY (attachment_id)
    REFERENCES attachment(id) ON DELETE CASCADE;

-- One open suggestion per company.
CREATE UNIQUE INDEX uq_deal_suggestion_open_company ON deal_suggestion (company_id) WHERE state = 'open';
CREATE INDEX idx_deal_suggestion_company ON deal_suggestion (company_id, decided_at);
CREATE INDEX idx_deal_suggestion_open ON deal_suggestion (created_at DESC, id DESC) WHERE state = 'open';
CREATE INDEX idx_deal_suggestion_pipeline ON deal_suggestion (pipeline_id);
CREATE INDEX idx_deal_suggestion_stage ON deal_suggestion (proposed_stage_id);
CREATE INDEX idx_deal_suggestion_accepted_deal ON deal_suggestion (accepted_deal_id) WHERE accepted_deal_id IS NOT NULL;
CREATE INDEX idx_deal_suggestion_decided_by ON deal_suggestion (decided_by) WHERE decided_by IS NOT NULL;
CREATE INDEX idx_deal_suggestion_evidence_suggestion ON deal_suggestion_evidence (suggestion_id);
CREATE INDEX idx_deal_suggestion_evidence_activity ON deal_suggestion_evidence (activity_id) WHERE activity_id IS NOT NULL;
CREATE INDEX idx_deal_suggestion_evidence_signal ON deal_suggestion_evidence (signal_id) WHERE signal_id IS NOT NULL;
CREATE INDEX idx_deal_suggestion_evidence_attachment ON deal_suggestion_evidence (attachment_id) WHERE attachment_id IS NOT NULL;

CREATE TRIGGER trg_deal_suggestion_updated
    BEFORE UPDATE ON deal_suggestion
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();
