-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The acceptance stamps go with the columns; the audit log keeps who accepted
-- each lead.
SET LOCAL lock_timeout = '3s';

DROP INDEX idx_lead_partner_company;
ALTER TABLE lead
    DROP CONSTRAINT lead_partner_acceptance_pairing,
    DROP CONSTRAINT lead_partner_company_id_fkey,
    DROP COLUMN partner_accepted_by,
    DROP COLUMN partner_accepted_at,
    DROP COLUMN partner_company_id;
