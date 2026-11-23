-- The constraints added NOT VALID by the file beside this one are validated here, in
-- their own transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '5s';

ALTER TABLE lead_manual_signal VALIDATE CONSTRAINT lead_manual_signal_set_by_fkey;
ALTER TABLE meeting_invitation VALIDATE CONSTRAINT meeting_invitation_host_user_id_fkey;
ALTER TABLE meeting_proposal VALIDATE CONSTRAINT meeting_proposal_host_user_id_fkey;
ALTER TABLE provider_connection VALIDATE CONSTRAINT provider_connection_connected_by_fkey;
ALTER TABLE provider_run VALIDATE CONSTRAINT provider_run_requested_by_fkey;
