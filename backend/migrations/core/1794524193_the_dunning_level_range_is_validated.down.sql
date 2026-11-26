-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Put the range check back unvalidated. Postgres has no ALTER that
-- un-validates, so it is dropped and re-added NOT VALID.
SET LOCAL lock_timeout = '3s';

ALTER TABLE finance_invoice
    DROP CONSTRAINT finance_invoice_dunning_level_range,
    ADD CONSTRAINT finance_invoice_dunning_level_range
        CHECK (dunning_level BETWEEN 0 AND 99) NOT VALID;
