SET LOCAL lock_timeout = '3s';
DROP TRIGGER notice_system_text_retire ON notice;
DROP TRIGGER activity_system_text_retire ON activity;
DROP FUNCTION retire_changed_system_text();
ALTER TABLE notice DROP COLUMN system_text;
ALTER TABLE activity DROP COLUMN system_text;
