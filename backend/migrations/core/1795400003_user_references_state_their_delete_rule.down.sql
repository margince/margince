-- Restores the user references to their earlier delete rules. A row whose user
-- was removed holds NULL, which SET NOT NULL refuses until it is dealt with.
SET LOCAL lock_timeout = '5s';

ALTER TABLE intro_request
    DROP CONSTRAINT intro_request_introducer_user_id_fkey,
    ADD CONSTRAINT intro_request_introducer_user_id_fkey FOREIGN KEY (introducer_user_id) REFERENCES app_user(id),
    DROP CONSTRAINT intro_request_requester_user_id_fkey,
    ADD CONSTRAINT intro_request_requester_user_id_fkey FOREIGN KEY (requester_user_id) REFERENCES app_user(id),
    DROP CONSTRAINT intro_request_suggested_user_id_fkey,
    ADD CONSTRAINT intro_request_suggested_user_id_fkey FOREIGN KEY (suggested_user_id) REFERENCES app_user(id);
ALTER TABLE lead_manual_signal
    DROP CONSTRAINT lead_manual_signal_set_by_fkey,
    ADD CONSTRAINT lead_manual_signal_set_by_fkey FOREIGN KEY (set_by) REFERENCES app_user(id);
ALTER TABLE meeting_invitation
    DROP CONSTRAINT meeting_invitation_host_user_id_fkey,
    ADD CONSTRAINT meeting_invitation_host_user_id_fkey FOREIGN KEY (host_user_id) REFERENCES app_user(id);
ALTER TABLE meeting_proposal
    DROP CONSTRAINT meeting_proposal_host_user_id_fkey,
    ADD CONSTRAINT meeting_proposal_host_user_id_fkey FOREIGN KEY (host_user_id) REFERENCES app_user(id);
ALTER TABLE provider_connection
    DROP CONSTRAINT provider_connection_connected_by_fkey,
    ADD CONSTRAINT provider_connection_connected_by_fkey FOREIGN KEY (connected_by) REFERENCES app_user(id);
ALTER TABLE provider_run
    DROP CONSTRAINT provider_run_requested_by_fkey,
    ADD CONSTRAINT provider_run_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES app_user(id);
ALTER TABLE report_definition
    DROP CONSTRAINT report_definition_owner_id_fkey,
    ADD CONSTRAINT report_definition_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES app_user(id);
ALTER TABLE report_definition_revision
    DROP CONSTRAINT report_definition_revision_created_by_fkey,
    ADD CONSTRAINT report_definition_revision_created_by_fkey FOREIGN KEY (created_by) REFERENCES app_user(id);
ALTER TABLE report_execution
    DROP CONSTRAINT report_execution_owner_id_fkey,
    ADD CONSTRAINT report_execution_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES app_user(id);
ALTER TABLE report_schedule
    DROP CONSTRAINT report_schedule_owner_id_fkey,
    ADD CONSTRAINT report_schedule_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES app_user(id);
ALTER TABLE reporting_framework_revision
    DROP CONSTRAINT reporting_framework_revision_created_by_fkey,
    ADD CONSTRAINT reporting_framework_revision_created_by_fkey FOREIGN KEY (created_by) REFERENCES app_user(id);
ALTER TABLE sales_initiative
    DROP CONSTRAINT sales_initiative_owner_id_fkey,
    ADD CONSTRAINT sales_initiative_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES app_user(id);
ALTER TABLE sales_target_revision
    DROP CONSTRAINT sales_target_revision_created_by_fkey,
    ADD CONSTRAINT sales_target_revision_created_by_fkey FOREIGN KEY (created_by) REFERENCES app_user(id);
ALTER TABLE sdr_handoff
    DROP CONSTRAINT sdr_handoff_assigned_to_fkey,
    ADD CONSTRAINT sdr_handoff_assigned_to_fkey FOREIGN KEY (assigned_to) REFERENCES app_user(id),
    DROP CONSTRAINT sdr_handoff_submitted_by_fkey,
    ADD CONSTRAINT sdr_handoff_submitted_by_fkey FOREIGN KEY (submitted_by) REFERENCES app_user(id);
ALTER TABLE weekly_plan_commitment
    DROP CONSTRAINT weekly_plan_commitment_manager_fkey,
    ADD CONSTRAINT weekly_plan_commitment_manager_fkey FOREIGN KEY (manager_user_id) REFERENCES app_user(id);
ALTER TABLE notification_digest_run
    DROP CONSTRAINT notification_digest_run_user_id_fkey,
    ADD CONSTRAINT notification_digest_run_user_id_fkey FOREIGN KEY (user_id) REFERENCES app_user(id) ON DELETE CASCADE;
ALTER TABLE brief_run
    DROP CONSTRAINT brief_run_user_fkey,
    ADD CONSTRAINT brief_run_user_fkey FOREIGN KEY (user_id) REFERENCES app_user(id) ON DELETE CASCADE;
ALTER TABLE brief_run ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE report_run
    DROP CONSTRAINT report_run_asked_by_fkey,
    ADD CONSTRAINT report_run_asked_by_fkey FOREIGN KEY (asked_by) REFERENCES app_user(id) ON DELETE CASCADE;
ALTER TABLE report_run ALTER COLUMN asked_by SET NOT NULL;
ALTER TABLE worklist_snapshot
    DROP CONSTRAINT worklist_snapshot_reader_id_fkey,
    ADD CONSTRAINT worklist_snapshot_reader_id_fkey FOREIGN KEY (reader_id) REFERENCES app_user(id) ON DELETE CASCADE;
ALTER TABLE worklist_snapshot ALTER COLUMN reader_id SET NOT NULL;
