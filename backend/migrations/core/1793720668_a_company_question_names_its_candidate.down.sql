-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE company_domain_disposition
    DROP COLUMN candidate_company_id,
    DROP COLUMN site_unreadable;
