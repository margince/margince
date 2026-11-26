SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_participant DROP COLUMN relinked_at;
