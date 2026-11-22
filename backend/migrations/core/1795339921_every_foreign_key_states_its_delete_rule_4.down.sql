-- Tables: report_definition, report_definition_revision, report_edition, report_execution, report_schedule.
-- Restores the foreign keys on these tables without a stated delete rule.
-- Rollback restores the rule for new rows only: the keys come back NOT VALID,
-- because a down has no later file to validate them in.
SET LOCAL lock_timeout = '5s';

ALTER TABLE report_definition
    DROP CONSTRAINT report_definition_audience_team_id_fkey,
    ADD CONSTRAINT report_definition_audience_team_id_fkey FOREIGN KEY (audience_team_id) REFERENCES team(id) NOT VALID,
    DROP CONSTRAINT report_definition_owner_id_fkey,
    ADD CONSTRAINT report_definition_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES app_user(id) NOT VALID;
ALTER TABLE report_definition_revision
    DROP CONSTRAINT report_definition_revision_created_by_fkey,
    ADD CONSTRAINT report_definition_revision_created_by_fkey FOREIGN KEY (created_by) REFERENCES app_user(id) NOT VALID,
    DROP CONSTRAINT report_definition_revision_report_id_fkey,
    ADD CONSTRAINT report_definition_revision_report_id_fkey FOREIGN KEY (report_id) REFERENCES report_definition(id) NOT VALID;
ALTER TABLE report_edition
    DROP CONSTRAINT report_edition_execution_id_fkey,
    ADD CONSTRAINT report_edition_execution_id_fkey FOREIGN KEY (execution_id) REFERENCES report_execution(id) NOT VALID,
    DROP CONSTRAINT report_edition_report_id_fkey,
    ADD CONSTRAINT report_edition_report_id_fkey FOREIGN KEY (report_id) REFERENCES report_definition(id) NOT VALID;
ALTER TABLE report_execution
    DROP CONSTRAINT report_execution_owner_id_fkey,
    ADD CONSTRAINT report_execution_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES app_user(id) NOT VALID,
    DROP CONSTRAINT report_execution_report_id_report_revision_fkey,
    ADD CONSTRAINT report_execution_report_id_report_revision_fkey FOREIGN KEY (report_id, report_revision) REFERENCES report_definition_revision(report_id, revision) NOT VALID,
    DROP CONSTRAINT report_execution_schedule_id_fkey,
    ADD CONSTRAINT report_execution_schedule_id_fkey FOREIGN KEY (schedule_id) REFERENCES report_schedule(id) NOT VALID;
ALTER TABLE report_schedule
    DROP CONSTRAINT report_schedule_owner_id_fkey,
    ADD CONSTRAINT report_schedule_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES app_user(id) NOT VALID,
    DROP CONSTRAINT report_schedule_report_id_report_revision_fkey,
    ADD CONSTRAINT report_schedule_report_id_report_revision_fkey FOREIGN KEY (report_id, report_revision) REFERENCES report_definition_revision(report_id, revision) NOT VALID;
