-- The constraints added NOT VALID by the file beside this one are validated here, in
-- their own transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '5s';

ALTER TABLE deal_product VALIDATE CONSTRAINT deal_product_product_id_fkey;
ALTER TABLE field_mask VALIDATE CONSTRAINT field_mask_maskable_fkey;
ALTER TABLE forecast_snapshot VALIDATE CONSTRAINT forecast_snapshot_pipeline_id_fkey;
ALTER TABLE intro_request VALIDATE CONSTRAINT intro_request_introducer_user_id_fkey;
ALTER TABLE intro_request VALIDATE CONSTRAINT intro_request_requester_user_id_fkey;
ALTER TABLE intro_request VALIDATE CONSTRAINT intro_request_suggested_user_id_fkey;
ALTER TABLE intro_request VALIDATE CONSTRAINT intro_request_through_contact_id_fkey;
ALTER TABLE lead VALIDATE CONSTRAINT lead_product_id_fkey;
