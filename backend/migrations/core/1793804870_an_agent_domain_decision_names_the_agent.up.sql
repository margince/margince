-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- An agent answering a domain question for its user stamps 'agent', not 'human'.
-- NOT VALID, and validated in the migration after this one: a plain ADD
-- CONSTRAINT scans the whole table under ACCESS EXCLUSIVE.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company_domain_disposition
    DROP CONSTRAINT company_domain_disposition_admission_source_check,
    ADD CONSTRAINT company_domain_disposition_admission_source_check
        CHECK (admission_source IS NULL OR admission_source IN ('verdict', 'heuristic', 'human', 'agent')) NOT VALID,
    DROP CONSTRAINT company_domain_disposition_source_check,
    ADD CONSTRAINT company_domain_disposition_source_check
        CHECK (source IS NULL OR source IN ('site_read', 'heuristic', 'human', 'agent')) NOT VALID;
