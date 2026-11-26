-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- Core search matches a ledger account by a LIKE on lower(account_code), the
-- kind's title column. This trigram index serves that match; without it core's
-- readiness probe (VerifyRecordKinds) reports the unit unready.

CREATE INDEX IF NOT EXISTS ext_finance_demo_ledger_account_code_trgm
    ON ext.ext_finance_demo_ledger_account USING gin (lower(account_code) public.gin_trgm_ops);
