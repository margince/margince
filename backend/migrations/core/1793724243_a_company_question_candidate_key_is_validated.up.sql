-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Its own file: VALIDATE takes only SHARE UPDATE EXCLUSIVE, which holds only
-- while no earlier statement in the transaction holds ACCESS EXCLUSIVE.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company_domain_disposition VALIDATE CONSTRAINT company_domain_disposition_candidate_company_id_fkey;
