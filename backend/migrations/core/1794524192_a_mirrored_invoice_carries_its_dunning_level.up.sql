-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A mirrored invoice carries the dunning level its accounting source states:
-- how many payment reminders it has sent. Null when the source does not say.
-- The range check is NOT VALID here and validated by the next migration.
SET LOCAL lock_timeout = '3s';

ALTER TABLE finance_invoice
    ADD COLUMN dunning_level smallint,
    ADD CONSTRAINT finance_invoice_dunning_level_range
        CHECK (dunning_level BETWEEN 0 AND 99) NOT VALID;
