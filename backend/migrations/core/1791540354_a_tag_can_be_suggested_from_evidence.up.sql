-- Tag suggestions: an admin marks a tag suggestible and describes what interest
-- looks like; a pass over captured mail and meeting notes proposes the tag on
-- the contact or company the evidence is filed under. Nothing is applied until
-- a person accepts. Evidence is one row per cited activity, so a reader's right
-- to see a suggestion is asked item by item.
SET LOCAL lock_timeout = '5s';

ALTER TABLE tag ADD COLUMN suggestible boolean DEFAULT false NOT NULL;
-- Validated in the file beside this one, outside this file's ACCESS EXCLUSIVE.
ALTER TABLE tag ADD CONSTRAINT tag_suggestible_is_described
    CHECK (NOT suggestible OR (description IS NOT NULL AND btrim(description) <> '')) NOT VALID;

CREATE TABLE tag_suggestion (
    id uuid DEFAULT uuidv7() NOT NULL,
    tag_id uuid NOT NULL,
    -- One branch per record type, so erasing the record takes the suggestion.
    contact_id uuid,
    company_id uuid,
    state text DEFAULT 'open' NOT NULL,
    -- How many evidence rows the suggestion was written with. The visibility
    -- rule compares it with the rows still present, so losing one to an
    -- erasure hides the suggestion rather than showing it to more readers.
    evidence_count integer NOT NULL,
    -- The newest evidence time. A dismissal re-arms only for evidence newer
    -- than the decision.
    evidence_through timestamptz NOT NULL,
    decided_by uuid,
    decided_at timestamptz,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT tag_suggestion_pkey PRIMARY KEY (id),
    CONSTRAINT tag_suggestion_one_record CHECK ((contact_id IS NULL) <> (company_id IS NULL)),
    CONSTRAINT tag_suggestion_state_check CHECK (state IN ('open', 'accepted', 'dismissed', 'superseded')),
    CONSTRAINT tag_suggestion_decided_shape CHECK ((state = 'open') = (decided_at IS NULL)),
    CONSTRAINT tag_suggestion_evidence_count_check CHECK (evidence_count > 0)
);

CREATE TABLE tag_suggestion_evidence (
    id uuid DEFAULT uuidv7() NOT NULL,
    suggestion_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    occurred_at timestamptz NOT NULL,

    CONSTRAINT tag_suggestion_evidence_pkey PRIMARY KEY (id),
    CONSTRAINT uq_tag_suggestion_evidence UNIQUE (suggestion_id, activity_id)
);

ALTER TABLE tag_suggestion
    ADD CONSTRAINT tag_suggestion_tag_id_fkey FOREIGN KEY (tag_id)
    REFERENCES tag(id) ON DELETE CASCADE;
ALTER TABLE tag_suggestion
    ADD CONSTRAINT tag_suggestion_contact_id_fkey FOREIGN KEY (contact_id)
    REFERENCES contact(id) ON DELETE CASCADE;
ALTER TABLE tag_suggestion
    ADD CONSTRAINT tag_suggestion_company_id_fkey FOREIGN KEY (company_id)
    REFERENCES company(id) ON DELETE CASCADE;
ALTER TABLE tag_suggestion
    ADD CONSTRAINT tag_suggestion_decided_by_fkey FOREIGN KEY (decided_by)
    REFERENCES app_user(id) ON DELETE SET NULL;
-- An evidence row goes with the activity it cites; evidence_count then hides
-- the suggestion.
ALTER TABLE tag_suggestion_evidence
    ADD CONSTRAINT tag_suggestion_evidence_suggestion_id_fkey FOREIGN KEY (suggestion_id)
    REFERENCES tag_suggestion(id) ON DELETE CASCADE;
ALTER TABLE tag_suggestion_evidence
    ADD CONSTRAINT tag_suggestion_evidence_activity_id_fkey FOREIGN KEY (activity_id)
    REFERENCES activity(id) ON DELETE CASCADE;

-- One open suggestion per tag and record.
CREATE UNIQUE INDEX uq_tag_suggestion_open_contact ON tag_suggestion (tag_id, contact_id)
    WHERE state = 'open' AND contact_id IS NOT NULL;
CREATE UNIQUE INDEX uq_tag_suggestion_open_company ON tag_suggestion (tag_id, company_id)
    WHERE state = 'open' AND company_id IS NOT NULL;
CREATE INDEX idx_tag_suggestion_contact ON tag_suggestion (contact_id, tag_id, decided_at)
    WHERE contact_id IS NOT NULL;
CREATE INDEX idx_tag_suggestion_company ON tag_suggestion (company_id, tag_id, decided_at)
    WHERE company_id IS NOT NULL;
CREATE INDEX idx_tag_suggestion_tag ON tag_suggestion (tag_id);
CREATE INDEX idx_tag_suggestion_open ON tag_suggestion (created_at DESC, id DESC) WHERE state = 'open';
CREATE INDEX idx_tag_suggestion_decided_by ON tag_suggestion (decided_by) WHERE decided_by IS NOT NULL;
CREATE INDEX idx_tag_suggestion_evidence_activity ON tag_suggestion_evidence (activity_id);

CREATE TRIGGER trg_tag_suggestion_updated
    BEFORE UPDATE ON tag_suggestion
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
