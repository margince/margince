-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A lead can name the partner that referred it, and record who accepted it for
-- that partner and when. Accepting a partner's first lead is one of the two
-- ways a partner is activated; the other is a won deal it sourced.
SET LOCAL lock_timeout = '3s';

ALTER TABLE lead
    ADD COLUMN partner_company_id  uuid,
    ADD COLUMN partner_accepted_at timestamptz,
    ADD COLUMN partner_accepted_by text;

-- Added NOT VALID and validated in the next file, so the scan does not hold
-- every lead write. The columns are new and empty, so no row can fail.
ALTER TABLE lead
    ADD CONSTRAINT lead_partner_company_id_fkey
        FOREIGN KEY (partner_company_id) REFERENCES company (id) ON DELETE SET NULL (partner_company_id) NOT VALID,
    ADD CONSTRAINT lead_partner_acceptance_pairing
        CHECK ((partner_accepted_at IS NULL) = (partner_accepted_by IS NULL)) NOT VALID;

CREATE INDEX idx_lead_partner_company ON lead (partner_company_id) WHERE partner_company_id IS NOT NULL;
