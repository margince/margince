SET LOCAL lock_timeout = '2s';
ALTER TABLE scheduled_send DROP COLUMN rearmed_at;
