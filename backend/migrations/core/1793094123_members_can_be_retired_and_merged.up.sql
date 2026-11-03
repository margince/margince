SET LOCAL lock_timeout = '3s';
ALTER TABLE app_user
    ADD COLUMN removed_at timestamptz,
    ADD COLUMN merged_into_user_id uuid,
    ADD CONSTRAINT app_user_merged_into_user_id_fkey FOREIGN KEY (merged_into_user_id) REFERENCES app_user(id) ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT app_user_merge_is_retired CHECK (merged_into_user_id IS NULL OR removed_at IS NOT NULL) NOT VALID,
    ADD CONSTRAINT app_user_removed_is_deactivated CHECK (removed_at IS NULL OR status = 'deactivated') NOT VALID,
    ADD CONSTRAINT app_user_does_not_merge_into_itself CHECK (merged_into_user_id IS NULL OR merged_into_user_id <> id) NOT VALID;
CREATE INDEX idx_app_user_merge_target ON app_user (merged_into_user_id) WHERE merged_into_user_id IS NOT NULL;
