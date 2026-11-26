SET LOCAL lock_timeout = '2s';
ALTER TABLE scheduled_send ADD COLUMN rearmed_at timestamptz;
