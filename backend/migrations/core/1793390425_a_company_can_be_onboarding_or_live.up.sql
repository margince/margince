-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A company can be onboarding (the contract is signed and set-up runs) or live
-- (the agreed first scope is in use), between opportunity and customer.
--
-- Added NOT VALID and validated in the next file. Every stored value passes,
-- because the list only grows.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company DROP CONSTRAINT company_lifecycle_check;
ALTER TABLE company
    ADD CONSTRAINT company_lifecycle_check
        CHECK (lifecycle IN ('unknown', 'target', 'prospect', 'opportunity', 'onboarding', 'live',
                             'customer', 'former_customer', 'disqualified')) NOT VALID;
