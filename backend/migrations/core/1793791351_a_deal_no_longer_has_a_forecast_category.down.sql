-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The column comes back empty: the categories it held are gone, and every deal
-- reads as having none.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal
    ADD COLUMN forecast_category text,
    ADD CONSTRAINT deal_forecast_category_check CHECK (
        forecast_category IS NULL OR forecast_category IN ('commit', 'best_case', 'pipeline', 'omitted')) NOT VALID;
