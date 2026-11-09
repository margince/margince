-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Put each check back present and unvalidated, as this migration found it.
-- Postgres has no ALTER that un-validates, so each is dropped and re-added.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal
    DROP CONSTRAINT deal_commercial_mode_check,
    ADD CONSTRAINT deal_commercial_mode_check
        CHECK (commercial_mode IN ('estimate', 'lines', 'legacy')) NOT VALID,
    DROP CONSTRAINT deal_commercial_revision_js_safe,
    ADD CONSTRAINT deal_commercial_revision_js_safe
        CHECK (commercial_revision >= 0 AND commercial_revision <= '9007199254740991'::bigint) NOT VALID;

ALTER TABLE deal_product
    DROP CONSTRAINT deal_product_source_check,
    ADD CONSTRAINT deal_product_source_check CHECK (source IN ('offer', 'manual', 'line')) NOT VALID,
    DROP CONSTRAINT deal_product_amount_currency_pair,
    ADD CONSTRAINT deal_product_amount_currency_pair CHECK (
        (amount_minor IS NULL OR currency IS NOT NULL)
        AND (known_amount_minor IS NULL OR currency IS NOT NULL)
        AND (currency IS NULL OR amount_minor IS NOT NULL OR known_amount_minor IS NOT NULL)) NOT VALID,
    DROP CONSTRAINT deal_product_known_amount_minor_check,
    ADD CONSTRAINT deal_product_known_amount_minor_check
        CHECK (known_amount_minor IS NULL OR known_amount_minor >= 0) NOT VALID,
    DROP CONSTRAINT deal_product_known_amount_minor_exact,
    ADD CONSTRAINT deal_product_known_amount_minor_exact
        CHECK (known_amount_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID,
    DROP CONSTRAINT deal_product_commercial_revision_js_safe,
    ADD CONSTRAINT deal_product_commercial_revision_js_safe
        CHECK (commercial_revision >= 0 AND commercial_revision <= '9007199254740991'::bigint) NOT VALID,
    DROP CONSTRAINT deal_product_line_shape,
    ADD CONSTRAINT deal_product_line_shape
        CHECK ((source = 'line') = (commercial_revision IS NOT NULL)) NOT VALID;

ALTER TABLE offer
    DROP CONSTRAINT offer_source_commercial_revision_js_safe,
    ADD CONSTRAINT offer_source_commercial_revision_js_safe
        CHECK (source_commercial_revision >= 0
               AND source_commercial_revision <= '9007199254740991'::bigint) NOT VALID;
