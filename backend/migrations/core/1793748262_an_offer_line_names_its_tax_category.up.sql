-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The tax category an offer line takes its rate from. A line with a category
-- reads its rate from the tax rate sheet, for the country of the offer's legal
-- entity, and reads it again on the day the offer is sent. A line whose rate
-- was typed has no category and keeps that rate. Every line written before
-- this column existed has none, so it keeps the rate it already holds.
SET LOCAL lock_timeout = '3s';

ALTER TABLE offer_line_item
    ADD COLUMN tax_category text,
    ADD CONSTRAINT offer_line_item_tax_category_check
        CHECK (tax_category IS NULL OR tax_category IN ('standard', 'reduced', 'exempt')) NOT VALID;
