SET LOCAL lock_timeout = '5s';

ALTER TABLE activity_participant DROP COLUMN IF EXISTS display_name;
