-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DROP TRIGGER IF EXISTS trg_company_fact_count_revision ON company_fact;
DROP FUNCTION IF EXISTS company_fact_count_revision();
DROP TABLE IF EXISTS company_fact_revision;
