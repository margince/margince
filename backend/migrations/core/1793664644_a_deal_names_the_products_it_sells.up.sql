SET LOCAL lock_timeout = '3s';

-- What kind of value a product is: software, implementation or other. Null
-- means nobody has classified it, which is different from `other`.
ALTER TABLE product ADD COLUMN component_kind text
    CONSTRAINT product_component_kind_check CHECK (component_kind IN ('software', 'implementation', 'other'));

-- The products a deal sells, with the value allocated to each. `offer` rows
-- are rewritten from the accepted offer's lines; one `manual` row set by hand
-- replaces every `offer` row of the deal. The amount is in the deal's offer
-- currency and null while the deal is unpriced for that product.
-- component_kind is the product's classification when the row was written,
-- so a later catalog edit does not rewrite what a closed deal sold.
CREATE TABLE deal_product (
    deal_id uuid NOT NULL REFERENCES deal(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES product(id),
    source text NOT NULL CONSTRAINT deal_product_source_check CHECK (source IN ('offer', 'manual')),
    offer_line_item_id uuid REFERENCES offer_line_item(id) ON DELETE SET NULL,
    amount_minor bigint CONSTRAINT deal_product_amount_minor_check CHECK (amount_minor IS NULL OR amount_minor >= 0),
    currency char(3) CONSTRAINT deal_product_currency_check CHECK (currency IS NULL OR currency ~ '^[A-Z]{3}$'),
    component_kind text CONSTRAINT deal_product_component_kind_check CHECK (component_kind IN ('software', 'implementation', 'other')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT deal_product_pkey PRIMARY KEY (deal_id, product_id),
    CONSTRAINT deal_product_amount_currency_pair CHECK ((amount_minor IS NULL) = (currency IS NULL))
);

CREATE INDEX deal_product_product_idx ON deal_product (product_id);

-- The product a lead is interested in, set at capture or by hand. A lead is
-- in a product's population through this column or through the product of
-- the deal it was promoted into, never through its company's other deals.
ALTER TABLE lead ADD COLUMN product_id uuid REFERENCES product(id);

CREATE INDEX lead_product_idx ON lead (product_id) WHERE product_id IS NOT NULL;
