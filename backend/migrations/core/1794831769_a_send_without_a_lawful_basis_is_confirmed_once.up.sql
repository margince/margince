SET LOCAL lock_timeout = '3s';

-- A user's confirmation that they read the warning for one recipient whose
-- country has no legitimate-interest basis and whose record holds no basis
-- for the use, and chose to send. Staging spends it once, on the delivery it
-- lets through, and records which. It expires unspent after a short while,
-- so it answers the send the user was writing and not a later one.
CREATE TABLE lawful_basis_confirmation (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    contact_id uuid REFERENCES contact(id) ON DELETE CASCADE,
    lead_id uuid REFERENCES lead(id) ON DELETE CASCADE,
    confirmed_by uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    confirmed_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    spent_at timestamptz,
    delivery_id uuid REFERENCES comms_outbound(id) ON DELETE SET NULL,
    CONSTRAINT lawful_basis_confirmation_one_subject CHECK ((contact_id IS NULL) <> (lead_id IS NULL)),
    CONSTRAINT lawful_basis_confirmation_window CHECK (expires_at > confirmed_at)
);
CREATE INDEX lawful_basis_confirmation_open_contact
    ON lawful_basis_confirmation (contact_id, confirmed_by) WHERE contact_id IS NOT NULL AND spent_at IS NULL;
CREATE INDEX lawful_basis_confirmation_open_lead
    ON lawful_basis_confirmation (lead_id, confirmed_by) WHERE lead_id IS NOT NULL AND spent_at IS NULL;
CREATE INDEX lawful_basis_confirmation_delivery
    ON lawful_basis_confirmation (delivery_id) WHERE delivery_id IS NOT NULL;
