SET LOCAL lock_timeout = '3s';

-- Somebody named on a company's website is told that we hold their details,
-- by the same privacy-notice mail a contact receives, before anything else is
-- sent to them. A website lead is a lead, not a contact, so the notice link
-- can now open for a lead as well.
--
-- Only the privacy notice: the record and consent links show or change a
-- contact's record, and a lead has none of that to show.
ALTER TABLE confirm_token ALTER COLUMN contact_id DROP NOT NULL;
ALTER TABLE confirm_token ADD COLUMN lead_id uuid REFERENCES lead(id) ON DELETE CASCADE;
ALTER TABLE confirm_token
    ADD CONSTRAINT confirm_token_one_subject
        CHECK ((contact_id IS NULL) <> (lead_id IS NULL)) NOT VALID;
ALTER TABLE confirm_token
    ADD CONSTRAINT confirm_token_lead_is_only_told
        CHECK (lead_id IS NULL OR kind = 'privacy_notice') NOT VALID;
CREATE INDEX ix_confirm_token_lead ON confirm_token (lead_id) WHERE lead_id IS NOT NULL;

-- One row per website lead the installation owed a notice: which wording, in
-- which language, carried by which delivery, and what became of it.
--
-- It names no address and no name. When the notice cannot be given (no
-- published address, an address another lead already holds, a refused send,
-- or a bounce) the lead is not kept, both references fall to NULL, and this
-- row is all that is left.
CREATE TABLE lead_privacy_notice (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    lead_id uuid REFERENCES lead(id) ON DELETE SET NULL,
    -- The accepted proposal the lead came from. It is emptied, not deleted,
    -- when the lead goes, so the decision a human made stays on record.
    approval_id uuid REFERENCES approval(id) ON DELETE SET NULL,
    delivery_id uuid REFERENCES comms_outbound(id) ON DELETE SET NULL,
    template_key text NOT NULL,
    template_version integer NOT NULL,
    -- NULL only where nothing was rendered: a lead with no published address.
    locale text,
    state text NOT NULL,
    failure text,
    queued_at timestamptz,
    failed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT lead_privacy_notice_state CHECK (state IN ('queued', 'failed')),
    CONSTRAINT lead_privacy_notice_failure
        CHECK (failure IS NULL OR failure IN ('no_address', 'collision', 'refused', 'bounced')),
    CONSTRAINT lead_privacy_notice_failed_shape
        CHECK ((state = 'failed') = (failure IS NOT NULL AND failed_at IS NOT NULL))
);

-- One notice per delivery: the bounce path finds its row by the delivery.
CREATE UNIQUE INDEX uq_lead_privacy_notice_delivery
    ON lead_privacy_notice (delivery_id) WHERE delivery_id IS NOT NULL;
-- One notice per lead, so a second accept of the same lead sends nothing more.
CREATE UNIQUE INDEX uq_lead_privacy_notice_lead
    ON lead_privacy_notice (lead_id) WHERE lead_id IS NOT NULL;
CREATE INDEX ix_lead_privacy_notice_approval
    ON lead_privacy_notice (approval_id) WHERE approval_id IS NOT NULL;
