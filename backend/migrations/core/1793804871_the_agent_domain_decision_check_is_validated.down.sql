-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Put the checks back the way the migration before this one left them: present,
-- unvalidated. Postgres has no ALTER that un-validates, so each is dropped and
-- added again NOT VALID, which takes ACCESS EXCLUSIVE without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company_domain_disposition
    DROP CONSTRAINT company_domain_disposition_admission_source_check,
    ADD CONSTRAINT company_domain_disposition_admission_source_check
        CHECK (admission_source IS NULL OR admission_source IN ('verdict', 'heuristic', 'human', 'agent')) NOT VALID,
    DROP CONSTRAINT company_domain_disposition_source_check,
    ADD CONSTRAINT company_domain_disposition_source_check
        CHECK (source IS NULL OR source IN ('site_read', 'heuristic', 'human', 'agent')) NOT VALID;
