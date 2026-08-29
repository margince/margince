-- A VAT ID is worth what the check behind it is worth.
--
-- An Impressum states a USt-IdNr under a statutory duty, and copying it into a
-- CRM records what a page said — not that the number is live, nor that it
-- belongs to the company whose page it was on. The EU's VIES service answers
-- both, and hands back a CONSULTATION NUMBER: the receipt that this
-- installation asked, on that date, about that number, and got that answer.
--
-- That receipt is the reason this table exists rather than a boolean on the
-- profile field. Under Art. 138 VAT Directive the consultation number is what
-- a business shows to say it checked its counterpart before treating a supply
-- as intra-community. A `valid = true` column proves nothing to anybody; a
-- consultation number, its date and the exact number consulted are evidence.
--
-- One row per organization: the CURRENT standing of that company's VAT ID,
-- replaced when re-checked. The history of checks is the audit log's, which
-- every write here also writes. Tenancy rides organization_id like every other
-- organization sidecar in this schema — no workspace_id column, no policy.
CREATE TABLE organization_vat_check (
    id uuid DEFAULT uuidv7() NOT NULL,
    organization_id uuid NOT NULL,
    -- The number as consulted, not as stored: a profile field edited after the
    -- check must not silently inherit its proof.
    vat_number text NOT NULL,
    -- What VIES said. `valid` and `invalid` are answers; `unavailable` is the
    -- service declining to answer (a member state's register offline), which
    -- is a fact about the lookup and never about the company.
    status text NOT NULL,
    -- The receipt. VIES returns one only for a check made with a requester's
    -- own VAT number, so an installation that has not stated its own gets a
    -- valid answer with no consultation number — the check still happened and
    -- is still worth recording.
    consultation_number text,
    -- Who VIES says the number belongs to, when it says. The point of the
    -- check is that a name disagreeing with the company on the record is the
    -- finding, so the answer is kept rather than compared away.
    registered_name text,
    registered_address text,
    checked_at timestamptz DEFAULT now() NOT NULL,
    captured_by text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT organization_vat_check_status_check
        CHECK (status IN ('valid', 'invalid', 'unavailable')),
    -- A consultation number is only meaningful beside a real answer. VIES
    -- issues none for a lookup it could not perform.
    CONSTRAINT organization_vat_check_receipt_needs_an_answer
        CHECK (consultation_number IS NULL OR status <> 'unavailable'),
    CONSTRAINT organization_vat_check_number_not_blank
        CHECK (btrim(vat_number) <> '')
);

ALTER TABLE organization_vat_check
    ADD CONSTRAINT organization_vat_check_pkey PRIMARY KEY (id);

-- The current standing, one row per company. A re-check updates in place
-- rather than accumulating rows nobody reads.
ALTER TABLE organization_vat_check
    ADD CONSTRAINT organization_vat_check_one_per_org UNIQUE (organization_id);

ALTER TABLE organization_vat_check
    ADD CONSTRAINT organization_vat_check_org_fk
    FOREIGN KEY (organization_id) REFERENCES organization (id) ON DELETE CASCADE;
