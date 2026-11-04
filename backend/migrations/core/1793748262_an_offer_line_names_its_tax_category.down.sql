-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Lines no longer name a tax category; each keeps the rate it holds.
SET LOCAL lock_timeout = '3s';

ALTER TABLE offer_line_item DROP COLUMN IF EXISTS tax_category;
