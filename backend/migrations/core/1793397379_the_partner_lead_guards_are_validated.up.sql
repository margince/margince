-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The validation of the guards the previous file added, in a transaction of its
-- own so writers pass while it scans. None can fail: the columns are new.
SET LOCAL lock_timeout = '3s';

ALTER TABLE lead VALIDATE CONSTRAINT lead_partner_company_id_fkey;
ALTER TABLE lead VALIDATE CONSTRAINT lead_partner_acceptance_pairing;
