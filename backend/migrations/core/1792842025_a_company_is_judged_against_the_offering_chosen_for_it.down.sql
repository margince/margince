-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Every company is judged against the main offering again.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_company_assess_against_entity;
ALTER TABLE company DROP COLUMN IF EXISTS assess_against_entity_id;
ALTER TABLE legal_entity
    DROP CONSTRAINT IF EXISTS legal_entity_offering_bounded,
    DROP CONSTRAINT IF EXISTS legal_entity_default_has_no_offering,
    DROP COLUMN IF EXISTS offer_summary,
    DROP COLUMN IF EXISTS icp,
    DROP COLUMN IF EXISTS value_proposition,
    DROP COLUMN IF EXISTS usp;
