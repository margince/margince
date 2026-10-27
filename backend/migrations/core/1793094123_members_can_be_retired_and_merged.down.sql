SET LOCAL lock_timeout = '3s';
ALTER TABLE app_user DROP COLUMN merged_into_user_id, DROP COLUMN removed_at;
