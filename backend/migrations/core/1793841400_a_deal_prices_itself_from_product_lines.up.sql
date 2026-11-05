-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A deal can hold its own product lines, and those lines then decide its
-- value. commercial_mode says which source decides: `estimate` is a value a
-- human typed, `lines` is the sum of the deal's lines, and `legacy` is every
-- deal that existed before this file, whose value keeps the meaning it had.
-- Existing rows take `legacy` from the column default, so no row is
-- rewritten. New deals then start as `estimate`.
--
-- commercial_revision counts the accepted changes to the lines. An offer made
-- from the deal names the revision it copied, so a later change is visible.
--
-- Every check on a table this file did not create is added NOT VALID and
-- validated by the next file, so no table is scanned under ACCESS EXCLUSIVE.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal
    ADD COLUMN commercial_mode text NOT NULL DEFAULT 'legacy',
    ADD COLUMN commercial_revision bigint NOT NULL DEFAULT 0,
    ADD CONSTRAINT deal_commercial_mode_check
        CHECK (commercial_mode IN ('estimate', 'lines', 'legacy')) NOT VALID,
    ADD CONSTRAINT deal_commercial_revision_js_safe
        CHECK (commercial_revision >= 0 AND commercial_revision <= '9007199254740991'::bigint) NOT VALID;

ALTER TABLE deal ALTER COLUMN commercial_mode SET DEFAULT 'estimate';

-- One product or custom item a deal sells. A product line copies the
-- product's name, unit, price, tax and billing once, when it is added, so a
-- later catalogue edit never reprices it. catalog_name keeps the product's
-- name at that moment beside the description a seller may edit. quantity and
-- unit_price_minor may be unknown, which makes the deal's value incomplete
-- instead of zero. A line carries a tax category or a typed rate, never both.
CREATE TABLE deal_line_item (
    id uuid NOT NULL DEFAULT uuidv7(),
    deal_id uuid NOT NULL,
    position integer NOT NULL,
    product_id uuid,
    catalog_name text,
    sku text,
    description text NOT NULL,
    unit text NOT NULL DEFAULT 'piece',
    quantity numeric(14,3),
    unit_price_minor bigint,
    currency currency_code NOT NULL,
    discount_pct numeric(5,2) NOT NULL DEFAULT 0,
    tax_rate numeric(5,2),
    tax_category text,
    billing_model text,
    billing_interval_months integer,
    interval_count integer,
    component_kind text,
    captured_by text NOT NULL,
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT deal_line_item_pkey PRIMARY KEY (id),
    CONSTRAINT deal_line_item_deal_id_fkey FOREIGN KEY (deal_id) REFERENCES deal(id) ON DELETE CASCADE,
    CONSTRAINT deal_line_item_product_id_fkey FOREIGN KEY (product_id) REFERENCES product(id),
    CONSTRAINT deal_line_item_position_unique UNIQUE (deal_id, position),
    CONSTRAINT deal_line_item_position_check CHECK (position >= 1),
    CONSTRAINT deal_line_item_description_present CHECK (btrim(description) <> '' AND length(description) <= 2000),
    CONSTRAINT deal_line_item_bounded CHECK (
        length(unit) <= 100 AND (catalog_name IS NULL OR length(catalog_name) <= 500)
        AND (sku IS NULL OR length(sku) <= 200)),
    CONSTRAINT deal_line_item_quantity_check CHECK (quantity IS NULL OR quantity > 0),
    CONSTRAINT deal_line_item_unit_price_minor_check CHECK (unit_price_minor IS NULL OR unit_price_minor >= 0),
    CONSTRAINT deal_line_item_unit_price_minor_js_safe
        CHECK (unit_price_minor >= '-9007199254740991'::bigint AND unit_price_minor <= '9007199254740991'::bigint),
    CONSTRAINT deal_line_item_discount_pct_check CHECK (discount_pct >= 0 AND discount_pct <= 100),
    CONSTRAINT deal_line_item_tax_rate_check CHECK (tax_rate IS NULL OR tax_rate >= 0),
    CONSTRAINT deal_line_item_tax_category_check
        CHECK (tax_category IS NULL OR tax_category IN ('standard', 'reduced', 'exempt')),
    CONSTRAINT deal_line_item_one_tax_source CHECK ((tax_rate IS NULL) <> (tax_category IS NULL)),
    CONSTRAINT deal_line_item_billing_model_check
        CHECK (billing_model IS NULL OR billing_model IN ('one_time', 'recurring')),
    CONSTRAINT deal_line_item_billing_interval_check
        CHECK (billing_interval_months IS NULL OR billing_interval_months IN (1, 3, 6, 12)),
    CONSTRAINT deal_line_item_interval_count_check CHECK (interval_count IS NULL OR interval_count > 0),
    CONSTRAINT deal_line_item_billing_shape CHECK (
        (billing_model IS NULL AND billing_interval_months IS NULL AND interval_count IS NULL)
        OR (billing_model = 'one_time' AND billing_interval_months IS NULL AND interval_count IS NULL)
        OR (billing_model = 'recurring' AND billing_interval_months IS NOT NULL)),
    CONSTRAINT deal_line_item_component_kind_check
        CHECK (component_kind IS NULL OR component_kind IN ('software', 'implementation', 'other')),
    CONSTRAINT deal_line_item_version_js_safe
        CHECK (version >= '-9007199254740991'::bigint AND version <= '9007199254740991'::bigint)
);

CREATE INDEX deal_line_item_product_idx ON deal_line_item (product_id) WHERE product_id IS NOT NULL;

-- A `line` allocation is the deal's own lines for one product, summed. Its
-- amount is null while any of those lines is incomplete, and known_amount_minor
-- then holds the part that is known. commercial_revision names the lines it
-- was summed from.
ALTER TABLE deal_product
    ADD COLUMN known_amount_minor bigint,
    ADD COLUMN commercial_revision bigint,
    DROP CONSTRAINT deal_product_source_check,
    ADD CONSTRAINT deal_product_source_check CHECK (source IN ('offer', 'manual', 'line')) NOT VALID,
    DROP CONSTRAINT deal_product_amount_currency_pair,
    ADD CONSTRAINT deal_product_amount_currency_pair CHECK (
        (amount_minor IS NULL OR currency IS NOT NULL)
        AND (known_amount_minor IS NULL OR currency IS NOT NULL)
        AND (currency IS NULL OR amount_minor IS NOT NULL OR known_amount_minor IS NOT NULL)) NOT VALID,
    ADD CONSTRAINT deal_product_known_amount_minor_check
        CHECK (known_amount_minor IS NULL OR known_amount_minor >= 0) NOT VALID,
    ADD CONSTRAINT deal_product_known_amount_minor_exact
        CHECK (known_amount_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID,
    ADD CONSTRAINT deal_product_commercial_revision_js_safe
        CHECK (commercial_revision >= 0 AND commercial_revision <= '9007199254740991'::bigint) NOT VALID,
    ADD CONSTRAINT deal_product_line_shape
        CHECK ((source = 'line') = (commercial_revision IS NOT NULL)) NOT VALID;

-- An offer made from a deal's lines names the revision it copied, and each of
-- its lines names the deal line it came from. The deal line may be removed
-- later. The offer keeps the id as a record of where its line came from.
ALTER TABLE offer
    ADD COLUMN source_commercial_revision bigint,
    ADD CONSTRAINT offer_source_commercial_revision_js_safe
        CHECK (source_commercial_revision >= 0
               AND source_commercial_revision <= '9007199254740991'::bigint) NOT VALID;

ALTER TABLE offer_line_item ADD COLUMN source_deal_line_id uuid;

CREATE TRIGGER trg_deal_line_item_updated
    BEFORE UPDATE ON deal_line_item
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();
