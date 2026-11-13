SET LOCAL lock_timeout = '2s';
ALTER TABLE scheduled_send DROP COLUMN scheduled_wall;
ALTER TABLE mail_draft DROP COLUMN scheduled_wall;
