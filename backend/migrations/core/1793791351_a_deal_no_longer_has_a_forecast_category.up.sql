-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A deal's own win probability replaced its forecast category, and nothing
-- reads or writes the column any more. Dropping a column changes the catalog
-- and rewrites no row. Frozen forecast rows keep their category as history.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal
    DROP CONSTRAINT deal_forecast_category_check,
    DROP COLUMN forecast_category;
