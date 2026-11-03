SET LOCAL lock_timeout = '3s';

-- Deleting an offer line sets the allocation's reference to it null, and finds
-- the allocations through this index.
CREATE INDEX deal_product_offer_line_item_idx ON deal_product (offer_line_item_id)
    WHERE offer_line_item_id IS NOT NULL;

-- A product is archived, never deleted, so a delete that reaches one a deal or a
-- lead names is refused instead of left to the default.
ALTER TABLE deal_product DROP CONSTRAINT deal_product_product_id_fkey,
    ADD CONSTRAINT deal_product_product_id_fkey FOREIGN KEY (product_id) REFERENCES product(id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE lead DROP CONSTRAINT lead_product_id_fkey,
    ADD CONSTRAINT lead_product_id_fkey FOREIGN KEY (product_id) REFERENCES product(id) ON DELETE RESTRICT NOT VALID;
