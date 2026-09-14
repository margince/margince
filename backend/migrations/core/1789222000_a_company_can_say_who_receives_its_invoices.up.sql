-- A company can say who receives its invoices, and in what capacity.
--
-- Billing contacts are a relationship, not a field on the company. There is no
-- one person who handles invoices: a customer typically names somebody the
-- invoice is sent to, somebody who approves it, and an accounts-payable mailbox
-- that pays it. Those are three different people as often as they are one, and
-- a single `billing_email` column on the company cannot say which of them it is.
--
-- So this reuses the relationship edge, with a new kind. The role column
-- already exists and already carries free text for the other kinds
-- (an employment's `cto`, a stakeholder's `champion`), but for this kind the
-- role is what makes the row mean anything — an edge saying only "this person
-- is a billing contact" does not say whether to mail them the invoice or ask
-- them to approve it. The CHECK below therefore makes it REQUIRED and bounded,
-- for this kind alone; every other kind keeps its free text untouched.
--
-- Nothing here connects to Finance's accounting mirror. Naming somebody a
-- recipient does not send them an invoice, does not create a contact in the
-- accounting source, and does not grant consent to mail them.

SET LOCAL lock_timeout = '3s';

ALTER TABLE relationship DROP CONSTRAINT relationship_kind_check;

ALTER TABLE relationship ADD CONSTRAINT relationship_kind_check
    CHECK (kind = ANY (ARRAY[
        'employment', 'deal_stakeholder', 'partner_of', 'referred_by',
        'co_sell_with', 'project_stakeholder', 'project_company', 'works_with',
        'billing_contact'
    ]));

-- The edge hangs between a contact and a company, exactly like an employment,
-- and carries no other endpoint. Written with the same `kind <> ... OR ...`
-- shape as its six siblings above it: an arm that is FALSE for every other kind
-- leaves them alone, and the second arm is only asked of this one.
ALTER TABLE relationship ADD CONSTRAINT rel_billing_contact_shape
    CHECK (
        kind <> 'billing_contact'
        OR (
            contact_id IS NOT NULL
            AND company_id IS NOT NULL
            AND deal_id IS NULL
            AND project_id IS NULL
            AND counterparty_contact_id IS NULL
            AND counterparty_company_id IS NULL
        )
    );

-- The role is mandatory and bounded for this kind.
--
-- `role IS NOT NULL AND role IN (...)` rather than `role IN (...)` alone: a
-- NULL role makes `role IN (...)` evaluate to NULL, and a CHECK that evaluates
-- to NULL PASSES. Written the short way, this constraint would admit exactly
-- the row it exists to refuse — a billing contact with no stated capacity.
ALTER TABLE relationship ADD CONSTRAINT rel_billing_contact_role
    CHECK (
        kind <> 'billing_contact'
        OR (role IS NOT NULL AND role IN ('recipient', 'approver', 'accounts_payable'))
    );

-- One live row per company, person and role. The same person may hold several
-- roles at one company (a small customer's office manager is often all three),
-- and the same person may be a billing contact at several companies, so the
-- role is part of the key rather than the company alone.
CREATE UNIQUE INDEX uq_rel_billing_contact
    ON relationship (company_id, contact_id, role)
    WHERE kind = 'billing_contact' AND archived_at IS NULL;

-- Both directions are read: the company's finance view lists its billing
-- contacts, and a person's page lists the companies they bill for.
CREATE INDEX rel_billing_contact_by_company
    ON relationship (company_id, role)
    WHERE kind = 'billing_contact' AND archived_at IS NULL;

CREATE INDEX rel_billing_contact_by_contact
    ON relationship (contact_id)
    WHERE kind = 'billing_contact' AND archived_at IS NULL;
