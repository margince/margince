-- Tables: reporting_framework_revision, sales_initiative, sales_target, sales_target_revision, sdr_handoff.
-- Foreign keys on these tables say what a parent delete does. RESTRICT is what the
-- implicit NO ACTION already did, written down; SET NULL is for an optional
-- reference the row outlives. Added NOT VALID and validated in the file beside this one.
SET LOCAL lock_timeout = '5s';

ALTER TABLE reporting_framework_revision
    DROP CONSTRAINT reporting_framework_revision_created_by_fkey,
    ADD CONSTRAINT reporting_framework_revision_created_by_fkey FOREIGN KEY (created_by) REFERENCES app_user(id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE sales_initiative
    DROP CONSTRAINT sales_initiative_owner_id_fkey,
    ADD CONSTRAINT sales_initiative_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES app_user(id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE sales_target
    DROP CONSTRAINT sales_target_pipeline_id_fkey,
    ADD CONSTRAINT sales_target_pipeline_id_fkey FOREIGN KEY (pipeline_id) REFERENCES pipeline(id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE sales_target_revision
    DROP CONSTRAINT sales_target_revision_created_by_fkey,
    ADD CONSTRAINT sales_target_revision_created_by_fkey FOREIGN KEY (created_by) REFERENCES app_user(id) ON DELETE RESTRICT NOT VALID,
    DROP CONSTRAINT sales_target_revision_target_id_fkey,
    ADD CONSTRAINT sales_target_revision_target_id_fkey FOREIGN KEY (target_id) REFERENCES sales_target(id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE sdr_handoff
    DROP CONSTRAINT sdr_handoff_assigned_to_fkey,
    ADD CONSTRAINT sdr_handoff_assigned_to_fkey FOREIGN KEY (assigned_to) REFERENCES app_user(id) ON DELETE SET NULL NOT VALID,
    DROP CONSTRAINT sdr_handoff_submitted_by_fkey,
    ADD CONSTRAINT sdr_handoff_submitted_by_fkey FOREIGN KEY (submitted_by) REFERENCES app_user(id) ON DELETE RESTRICT NOT VALID;
