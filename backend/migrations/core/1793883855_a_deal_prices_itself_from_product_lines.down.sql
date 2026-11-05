-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE offer_line_item DROP COLUMN source_deal_line_id;
ALTER TABLE offer
    DROP CONSTRAINT offer_source_commercial_revision_js_safe,
    DROP COLUMN source_commercial_revision;

DELETE FROM deal_product WHERE source = 'line';
ALTER TABLE deal_product
    DROP CONSTRAINT deal_product_line_shape,
    DROP CONSTRAINT deal_product_commercial_revision_js_safe,
    DROP CONSTRAINT deal_product_known_amount_minor_exact,
    DROP CONSTRAINT deal_product_known_amount_minor_check,
    DROP CONSTRAINT deal_product_amount_currency_pair,
    ADD CONSTRAINT deal_product_amount_currency_pair CHECK ((amount_minor IS NULL) = (currency IS NULL)) NOT VALID,
    DROP CONSTRAINT deal_product_source_check,
    ADD CONSTRAINT deal_product_source_check CHECK (source IN ('offer', 'manual')) NOT VALID,
    DROP COLUMN commercial_revision,
    DROP COLUMN known_amount_minor;

DROP TABLE deal_line_item;

ALTER TABLE deal
    DROP CONSTRAINT deal_commercial_revision_js_safe,
    DROP CONSTRAINT deal_commercial_mode_check,
    DROP COLUMN commercial_revision,
    DROP COLUMN commercial_mode;
