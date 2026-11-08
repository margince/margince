SET LOCAL lock_timeout = '3s';

DROP INDEX lead_product_idx;
ALTER TABLE lead DROP COLUMN product_id;
DROP TABLE deal_product;
ALTER TABLE product DROP COLUMN component_kind;
