-- Tables: voice_build, voice_learning_signal, voice_profile_delta, voice_profile_version, weekly_plan_commitment.
-- Foreign keys on these tables say what a parent delete does. RESTRICT is what the
-- implicit NO ACTION already did, written down; SET NULL is for an optional
-- reference the row outlives. Added NOT VALID and validated in the file beside this one.
SET LOCAL lock_timeout = '5s';

ALTER TABLE voice_build
    DROP CONSTRAINT voice_build_result_version_fkey,
    ADD CONSTRAINT voice_build_result_version_fkey FOREIGN KEY (voice_profile_id, result_version) REFERENCES voice_profile_version(voice_profile_id, profile_version) ON DELETE RESTRICT NOT VALID;
ALTER TABLE voice_learning_signal
    DROP CONSTRAINT voice_learning_signal_version_fkey,
    ADD CONSTRAINT voice_learning_signal_version_fkey FOREIGN KEY (voice_profile_id, profile_version) REFERENCES voice_profile_version(voice_profile_id, profile_version) ON DELETE RESTRICT NOT VALID;
ALTER TABLE voice_profile_delta
    DROP CONSTRAINT voice_profile_delta_from_fkey,
    ADD CONSTRAINT voice_profile_delta_from_fkey FOREIGN KEY (voice_profile_id, from_version) REFERENCES voice_profile_version(voice_profile_id, profile_version) ON DELETE RESTRICT NOT VALID,
    DROP CONSTRAINT voice_profile_delta_to_fkey,
    ADD CONSTRAINT voice_profile_delta_to_fkey FOREIGN KEY (voice_profile_id, to_version) REFERENCES voice_profile_version(voice_profile_id, profile_version) ON DELETE RESTRICT NOT VALID;
ALTER TABLE voice_profile_version
    DROP CONSTRAINT voice_profile_version_predecessor_fkey,
    ADD CONSTRAINT voice_profile_version_predecessor_fkey FOREIGN KEY (voice_profile_id, predecessor_version) REFERENCES voice_profile_version(voice_profile_id, profile_version) ON DELETE RESTRICT NOT VALID;
ALTER TABLE weekly_plan_commitment
    DROP CONSTRAINT weekly_plan_commitment_manager_fkey,
    ADD CONSTRAINT weekly_plan_commitment_manager_fkey FOREIGN KEY (manager_user_id) REFERENCES app_user(id) ON DELETE RESTRICT NOT VALID;
