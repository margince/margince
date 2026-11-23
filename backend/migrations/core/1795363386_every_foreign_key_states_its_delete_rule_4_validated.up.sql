-- The constraints added NOT VALID by the file beside this one are validated here, in
-- their own transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '5s';

ALTER TABLE report_definition VALIDATE CONSTRAINT report_definition_audience_team_id_fkey;
ALTER TABLE report_definition VALIDATE CONSTRAINT report_definition_owner_id_fkey;
ALTER TABLE report_definition_revision VALIDATE CONSTRAINT report_definition_revision_created_by_fkey;
ALTER TABLE report_definition_revision VALIDATE CONSTRAINT report_definition_revision_report_id_fkey;
ALTER TABLE report_edition VALIDATE CONSTRAINT report_edition_execution_id_fkey;
ALTER TABLE report_edition VALIDATE CONSTRAINT report_edition_report_id_fkey;
ALTER TABLE report_execution VALIDATE CONSTRAINT report_execution_owner_id_fkey;
ALTER TABLE report_execution VALIDATE CONSTRAINT report_execution_report_id_report_revision_fkey;
ALTER TABLE report_execution VALIDATE CONSTRAINT report_execution_schedule_id_fkey;
ALTER TABLE report_schedule VALIDATE CONSTRAINT report_schedule_owner_id_fkey;
ALTER TABLE report_schedule VALIDATE CONSTRAINT report_schedule_report_id_report_revision_fkey;
