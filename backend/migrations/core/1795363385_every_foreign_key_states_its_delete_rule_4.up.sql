-- Tables: report_definition, report_definition_revision, report_edition, report_execution, report_schedule.
-- Foreign keys on these tables say what a parent delete does. RESTRICT is what the
-- implicit NO ACTION already did, written down; SET NULL is for an optional
-- reference the row outlives. Added NOT VALID and validated in the file beside this one.
SET LOCAL lock_timeout = '5s';

ALTER TABLE report_definition
    DROP CONSTRAINT report_definition_audience_team_id_fkey,
    ADD CONSTRAINT report_definition_audience_team_id_fkey FOREIGN KEY (audience_team_id) REFERENCES team(id) ON DELETE RESTRICT NOT VALID,
    DROP CONSTRAINT report_definition_owner_id_fkey,
    ADD CONSTRAINT report_definition_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES app_user(id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE report_definition_revision
    DROP CONSTRAINT report_definition_revision_created_by_fkey,
    ADD CONSTRAINT report_definition_revision_created_by_fkey FOREIGN KEY (created_by) REFERENCES app_user(id) ON DELETE RESTRICT NOT VALID,
    DROP CONSTRAINT report_definition_revision_report_id_fkey,
    ADD CONSTRAINT report_definition_revision_report_id_fkey FOREIGN KEY (report_id) REFERENCES report_definition(id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE report_edition
    DROP CONSTRAINT report_edition_execution_id_fkey,
    ADD CONSTRAINT report_edition_execution_id_fkey FOREIGN KEY (execution_id) REFERENCES report_execution(id) ON DELETE RESTRICT NOT VALID,
    DROP CONSTRAINT report_edition_report_id_fkey,
    ADD CONSTRAINT report_edition_report_id_fkey FOREIGN KEY (report_id) REFERENCES report_definition(id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE report_execution
    DROP CONSTRAINT report_execution_owner_id_fkey,
    ADD CONSTRAINT report_execution_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES app_user(id) ON DELETE RESTRICT NOT VALID,
    DROP CONSTRAINT report_execution_report_id_report_revision_fkey,
    ADD CONSTRAINT report_execution_report_id_report_revision_fkey FOREIGN KEY (report_id, report_revision) REFERENCES report_definition_revision(report_id, revision) ON DELETE RESTRICT NOT VALID,
    DROP CONSTRAINT report_execution_schedule_id_fkey,
    ADD CONSTRAINT report_execution_schedule_id_fkey FOREIGN KEY (schedule_id) REFERENCES report_schedule(id) ON DELETE SET NULL NOT VALID;
ALTER TABLE report_schedule
    DROP CONSTRAINT report_schedule_owner_id_fkey,
    ADD CONSTRAINT report_schedule_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES app_user(id) ON DELETE RESTRICT NOT VALID,
    DROP CONSTRAINT report_schedule_report_id_report_revision_fkey,
    ADD CONSTRAINT report_schedule_report_id_report_revision_fkey FOREIGN KEY (report_id, report_revision) REFERENCES report_definition_revision(report_id, revision) ON DELETE RESTRICT NOT VALID;
