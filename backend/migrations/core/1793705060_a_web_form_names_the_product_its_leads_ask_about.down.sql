SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS web_form_product_idx;
ALTER TABLE web_form DROP COLUMN IF EXISTS product_id;
