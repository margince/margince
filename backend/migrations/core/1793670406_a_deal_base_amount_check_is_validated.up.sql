SET LOCAL lock_timeout = '3s';

ALTER TABLE deal VALIDATE CONSTRAINT deal_amount_minor_base_js_safe;
