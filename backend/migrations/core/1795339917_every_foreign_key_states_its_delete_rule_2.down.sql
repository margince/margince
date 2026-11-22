-- Tables: deal_product, field_mask, forecast_snapshot, intro_request, lead.
-- Restores the foreign keys on these tables without a stated delete rule.
SET LOCAL lock_timeout = '5s';

ALTER TABLE deal_product
    DROP CONSTRAINT deal_product_product_id_fkey,
    ADD CONSTRAINT deal_product_product_id_fkey FOREIGN KEY (product_id) REFERENCES product(id);
ALTER TABLE field_mask
    DROP CONSTRAINT field_mask_maskable_fkey,
    ADD CONSTRAINT field_mask_maskable_fkey FOREIGN KEY (object, field) REFERENCES maskable_field(object, field);
ALTER TABLE forecast_snapshot
    DROP CONSTRAINT forecast_snapshot_pipeline_id_fkey,
    ADD CONSTRAINT forecast_snapshot_pipeline_id_fkey FOREIGN KEY (pipeline_id) REFERENCES pipeline(id);
ALTER TABLE intro_request
    DROP CONSTRAINT intro_request_introducer_user_id_fkey,
    ADD CONSTRAINT intro_request_introducer_user_id_fkey FOREIGN KEY (introducer_user_id) REFERENCES app_user(id),
    DROP CONSTRAINT intro_request_requester_user_id_fkey,
    ADD CONSTRAINT intro_request_requester_user_id_fkey FOREIGN KEY (requester_user_id) REFERENCES app_user(id),
    DROP CONSTRAINT intro_request_suggested_user_id_fkey,
    ADD CONSTRAINT intro_request_suggested_user_id_fkey FOREIGN KEY (suggested_user_id) REFERENCES app_user(id),
    DROP CONSTRAINT intro_request_through_contact_id_fkey,
    ADD CONSTRAINT intro_request_through_contact_id_fkey FOREIGN KEY (through_contact_id) REFERENCES contact(id);
ALTER TABLE lead
    DROP CONSTRAINT lead_product_id_fkey,
    ADD CONSTRAINT lead_product_id_fkey FOREIGN KEY (product_id) REFERENCES product(id);
