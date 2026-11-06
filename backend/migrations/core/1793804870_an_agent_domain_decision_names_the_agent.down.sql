-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Narrowing back refuses a later write of 'agent', and NOT VALID lets the rows
-- an agent already wrote stand, because they exist.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company_domain_disposition
    DROP CONSTRAINT company_domain_disposition_admission_source_check,
    ADD CONSTRAINT company_domain_disposition_admission_source_check
        CHECK (admission_source IS NULL OR admission_source IN ('verdict', 'heuristic', 'human')) NOT VALID,
    DROP CONSTRAINT company_domain_disposition_source_check,
    ADD CONSTRAINT company_domain_disposition_source_check
        CHECK (source IS NULL OR source IN ('site_read', 'heuristic', 'human')) NOT VALID;
