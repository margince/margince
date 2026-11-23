-- Tables: lead_manual_signal, meeting_invitation, meeting_proposal, provider_connection, provider_run.
-- Restores the foreign keys on these tables without a stated delete rule.
-- Rollback restores the rule for new rows only: the keys come back NOT VALID,
-- because a down has no later file to validate them in.
SET LOCAL lock_timeout = '5s';

ALTER TABLE lead_manual_signal
    DROP CONSTRAINT lead_manual_signal_set_by_fkey,
    ADD CONSTRAINT lead_manual_signal_set_by_fkey FOREIGN KEY (set_by) REFERENCES app_user(id) NOT VALID;
ALTER TABLE meeting_invitation
    DROP CONSTRAINT meeting_invitation_host_user_id_fkey,
    ADD CONSTRAINT meeting_invitation_host_user_id_fkey FOREIGN KEY (host_user_id) REFERENCES app_user(id) NOT VALID;
ALTER TABLE meeting_proposal
    DROP CONSTRAINT meeting_proposal_host_user_id_fkey,
    ADD CONSTRAINT meeting_proposal_host_user_id_fkey FOREIGN KEY (host_user_id) REFERENCES app_user(id) NOT VALID;
ALTER TABLE provider_connection
    DROP CONSTRAINT provider_connection_connected_by_fkey,
    ADD CONSTRAINT provider_connection_connected_by_fkey FOREIGN KEY (connected_by) REFERENCES app_user(id) NOT VALID;
ALTER TABLE provider_run
    DROP CONSTRAINT provider_run_requested_by_fkey,
    ADD CONSTRAINT provider_run_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES app_user(id) NOT VALID;
