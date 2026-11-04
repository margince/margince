-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- A row holding an extension's type cannot satisfy the narrow check, so it goes
-- first. The type lives on only in the audit log.
DELETE FROM company_relationship_type WHERE relationship_type LIKE 'ext:%';

ALTER TABLE company_relationship_type
    DROP CONSTRAINT company_relationship_type_relationship_type_check;

ALTER TABLE company_relationship_type
    ADD CONSTRAINT company_relationship_type_relationship_type_check CHECK (
        relationship_type IN ('customer', 'partner', 'supplier', 'investor', 'portfolio_company', 'competitor', 'other')
    ) NOT VALID;
