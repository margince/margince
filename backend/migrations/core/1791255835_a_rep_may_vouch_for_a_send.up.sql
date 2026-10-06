SET LOCAL lock_timeout = '5s';

-- A rep vouching that we may write to a contact the engine refused for lack of
-- evidence. This is NOT consent and NOT a lawful basis: it sits beside the
-- refusal and only outranks a MACHINE-level, non-absolute one. A later subject
-- stop still wins, enforced by the gate's ordering, not by this row.
CREATE TABLE communication_override (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    contact_id uuid REFERENCES contact(id) ON DELETE CASCADE,
    lead_id uuid REFERENCES lead(id) ON DELETE CASCADE,
    category text NOT NULL,
    -- A vouch must be explainable; unlike a stop, the reason is mandatory at the
    -- door (enforced there, stored here).
    reason text NOT NULL,
    -- WHO DECIDED, from the principal and never the body. Only a seat writes one,
    -- so the vocabulary is the two seat levels; machine and subject cannot.
    decided_by_level text NOT NULL,
    captured_by text NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    -- Merge provenance, mirroring communication_suppression.carried_from.
    carried_from uuid REFERENCES communication_override(id) ON DELETE SET NULL,
    -- EXACTLY ONE subject, never both: the Allow door is contact-only and a
    -- carry sets exactly one of contact_id/lead_id, so a row naming both is a
    -- writer bug this refuses at the table rather than letting a subject-keyed
    -- read match a subject it was never meant to.
    CONSTRAINT communication_override_names_a_target
        CHECK ((contact_id IS NULL) <> (lead_id IS NULL)),
    CONSTRAINT communication_override_decided_by_level
        CHECK (decided_by_level = ANY (ARRAY['user'::text, 'admin'::text])),
    CONSTRAINT communication_override_category
        CHECK (category = ANY (ARRAY[
            'reply_to_inbound'::text, 'requested_followup'::text,
            'precontract_quote'::text, 'active_deal_followup'::text,
            'customer_service'::text, 'account_notice'::text,
            'contract_notice'::text, 'invoice_or_payment'::text,
            'security_notice'::text, 'privacy_notice'::text,
            'record_confirmation'::text, 'consent_confirmation'::text,
            'optout_confirmation'::text, 'marketing'::text
        ]))
);

CREATE INDEX communication_override_live_contact
    ON communication_override (contact_id, category) WHERE revoked_at IS NULL;
CREATE INDEX communication_override_live_lead
    ON communication_override (lead_id, category) WHERE revoked_at IS NULL;

-- The live indexes above are partial on revoked_at, which a cascade cannot use:
-- deleting the subject must find every override naming it, revoked ones included.
CREATE INDEX communication_override_contact_cascade
    ON communication_override (contact_id) WHERE contact_id IS NOT NULL;
CREATE INDEX communication_override_lead_cascade
    ON communication_override (lead_id) WHERE lead_id IS NOT NULL;

-- Three recursive walks join on carried_from — the family lock on revoke and
-- on carry, and the revoke itself — and the carry runs inside the merge
-- transaction. Partial on IS NOT NULL: a root carries NULL here and is never
-- the join's target, so indexing it would only widen the index.
CREATE INDEX communication_override_carried_from
    ON communication_override (carried_from) WHERE carried_from IS NOT NULL;
