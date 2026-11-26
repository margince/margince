-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- One row per ledger account the demo source knows. It is the reference table
-- for a record kind: core reads its `id` and `archived_at` to link, tag and list
-- the record, and the unique index over `account_code` is the natural key the
-- declaration names.

CREATE TABLE ext.ext_finance_demo_ledger_account (
    id           uuid        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    account_code text        NOT NULL CHECK (length(account_code) > 0),
    name         text        NOT NULL CHECK (length(name) > 0),
    archived_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ext_finance_demo_ledger_account_one_per_code UNIQUE (account_code)
);

-- The app role runs core's reads of this table. The grant is conditional for the
-- same reason core's app-role grants are: a throwaway database applying this
-- migration under its owning role alone has no margince_app role at all.
--
-- If the grant did not land, core's readiness probe (VerifyRecordKinds) reports
-- the unit unready and names ext_finance_demo_ledger_account.
DO $$
BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'margince_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_finance_demo_ledger_account TO margince_app;
  END IF;
END $$;
