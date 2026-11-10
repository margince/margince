-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The checks added by the migration before this one are validated here, under
-- SHARE UPDATE EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company_domain_disposition VALIDATE CONSTRAINT company_domain_disposition_admission_source_check;
ALTER TABLE company_domain_disposition VALIDATE CONSTRAINT company_domain_disposition_source_check;
