SET LOCAL lock_timeout = '3s';

-- The product every lead a web form captures is interested in, as the form's
-- lead origin and owner are. Null captures leads with no product.
ALTER TABLE web_form ADD COLUMN product_id uuid REFERENCES product(id);

CREATE INDEX web_form_product_idx ON web_form (product_id) WHERE product_id IS NOT NULL;
