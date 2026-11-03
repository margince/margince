-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- No tax rate sheet and no product categories. The role grants stay: an
-- object the code does not know grants nothing, and Parse drops it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE product DROP COLUMN IF EXISTS tax_category;
DROP TABLE IF EXISTS tax_rate;
DROP FUNCTION IF EXISTS tax_rate_refuse_change();
