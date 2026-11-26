-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The finance mirror stops carrying a dunning level.
SET LOCAL lock_timeout = '3s';

ALTER TABLE finance_invoice
    DROP CONSTRAINT finance_invoice_dunning_level_range,
    DROP COLUMN dunning_level;
