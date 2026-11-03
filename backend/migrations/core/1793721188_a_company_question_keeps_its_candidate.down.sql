-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE company_domain_disposition
    DROP COLUMN site_unread,
    DROP COLUMN candidate_name,
    DROP COLUMN candidate_company_id;
