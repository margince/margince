-- The constraints added NOT VALID by the file beside this one are validated here, in
-- their own transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '5s';

ALTER TABLE reporting_framework_revision VALIDATE CONSTRAINT reporting_framework_revision_created_by_fkey;
ALTER TABLE sales_initiative VALIDATE CONSTRAINT sales_initiative_owner_id_fkey;
ALTER TABLE sales_target VALIDATE CONSTRAINT sales_target_pipeline_id_fkey;
ALTER TABLE sales_target_revision VALIDATE CONSTRAINT sales_target_revision_created_by_fkey;
ALTER TABLE sales_target_revision VALIDATE CONSTRAINT sales_target_revision_target_id_fkey;
ALTER TABLE sdr_handoff VALIDATE CONSTRAINT sdr_handoff_assigned_to_fkey;
ALTER TABLE sdr_handoff VALIDATE CONSTRAINT sdr_handoff_submitted_by_fkey;
