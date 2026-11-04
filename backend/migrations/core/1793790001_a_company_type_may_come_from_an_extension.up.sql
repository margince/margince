-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A company type is one of core's seven words or `ext:<unit>:<type>`, the form an
-- extension's own type takes. Which `<unit>:<type>` pairs exist is the composed
-- set's to say, so the database holds only the shape and the API holds the set.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company_relationship_type
    DROP CONSTRAINT company_relationship_type_relationship_type_check;

ALTER TABLE company_relationship_type
    ADD CONSTRAINT company_relationship_type_relationship_type_check CHECK (
        relationship_type IN ('customer', 'partner', 'supplier', 'investor', 'portfolio_company', 'competitor', 'other')
        OR relationship_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z0-9]+(_[a-z0-9]+)*$'
    ) NOT VALID;
