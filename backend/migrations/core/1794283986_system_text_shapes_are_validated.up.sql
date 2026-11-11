SET LOCAL lock_timeout = '3s';
ALTER TABLE activity VALIDATE CONSTRAINT activity_system_text_shape;
ALTER TABLE notice VALIDATE CONSTRAINT notice_system_text_shape;
