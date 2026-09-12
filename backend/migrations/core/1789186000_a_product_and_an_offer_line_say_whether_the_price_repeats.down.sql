SET LOCAL lock_timeout = '3s';

ALTER TABLE deal DROP CONSTRAINT deal_arr_source_same_deal;
ALTER TABLE deal DROP COLUMN arr_source_offer_id;
ALTER TABLE offer DROP CONSTRAINT offer_deal_id_id_key;

ALTER TABLE offer_line_item DROP CONSTRAINT oli_interval_count_check;
ALTER TABLE offer_line_item DROP CONSTRAINT oli_billing_shape;
ALTER TABLE offer_line_item DROP CONSTRAINT oli_billing_interval_check;
ALTER TABLE offer_line_item DROP CONSTRAINT oli_billing_model_check;
ALTER TABLE offer_line_item DROP COLUMN interval_count;
ALTER TABLE offer_line_item DROP COLUMN billing_interval_months;
ALTER TABLE offer_line_item DROP COLUMN billing_model;

ALTER TABLE product DROP CONSTRAINT product_billing_interval_check;
ALTER TABLE product DROP CONSTRAINT product_billing_shape;
ALTER TABLE product DROP CONSTRAINT product_billing_model_check;
ALTER TABLE product DROP COLUMN billing_interval_months;
ALTER TABLE product DROP COLUMN billing_model;
