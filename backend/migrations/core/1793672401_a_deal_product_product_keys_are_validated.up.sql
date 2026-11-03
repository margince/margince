SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_product VALIDATE CONSTRAINT deal_product_product_id_fkey;
ALTER TABLE lead VALIDATE CONSTRAINT lead_product_id_fkey;
