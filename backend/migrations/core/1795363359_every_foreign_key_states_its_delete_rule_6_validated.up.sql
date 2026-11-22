-- The constraints added NOT VALID by the file beside this one are validated here, in
-- their own transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '5s';

ALTER TABLE voice_build VALIDATE CONSTRAINT voice_build_result_version_fkey;
ALTER TABLE voice_learning_signal VALIDATE CONSTRAINT voice_learning_signal_version_fkey;
ALTER TABLE voice_profile_delta VALIDATE CONSTRAINT voice_profile_delta_from_fkey;
ALTER TABLE voice_profile_delta VALIDATE CONSTRAINT voice_profile_delta_to_fkey;
ALTER TABLE voice_profile_version VALIDATE CONSTRAINT voice_profile_version_predecessor_fkey;
ALTER TABLE weekly_plan_commitment VALIDATE CONSTRAINT weekly_plan_commitment_manager_fkey;
