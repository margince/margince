SET LOCAL lock_timeout = '3s';
ALTER TABLE mail_draft DROP COLUMN editor_state;
