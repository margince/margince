SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user DROP COLUMN IF EXISTS greeting_name;
