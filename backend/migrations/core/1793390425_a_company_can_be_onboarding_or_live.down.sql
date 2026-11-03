-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The old list has no onboarding or live, and both are accounts we already
-- sell to, so they go back to customer.
SET LOCAL lock_timeout = '3s';

UPDATE company SET lifecycle = 'customer' WHERE lifecycle IN ('onboarding', 'live');

ALTER TABLE company DROP CONSTRAINT company_lifecycle_check;
ALTER TABLE company
    ADD CONSTRAINT company_lifecycle_check
        CHECK (lifecycle IN ('unknown', 'target', 'prospect', 'opportunity', 'customer',
                             'former_customer', 'disqualified')) NOT VALID;
