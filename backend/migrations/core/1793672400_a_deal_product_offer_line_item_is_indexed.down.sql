SET LOCAL lock_timeout = '3s';

DROP INDEX deal_product_offer_line_item_idx;
ALTER TABLE lead DROP CONSTRAINT lead_product_id_fkey,
    ADD CONSTRAINT lead_product_id_fkey FOREIGN KEY (product_id) REFERENCES product(id) NOT VALID;
ALTER TABLE deal_product DROP CONSTRAINT deal_product_product_id_fkey,
    ADD CONSTRAINT deal_product_product_id_fkey FOREIGN KEY (product_id) REFERENCES product(id) NOT VALID;
