SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user DROP COLUMN last_active_at;
