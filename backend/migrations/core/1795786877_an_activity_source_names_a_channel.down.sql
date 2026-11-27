SET LOCAL lock_timeout = '3s';

DROP TRIGGER activity_source_channel ON activity;
DROP FUNCTION move_activity_source_onto_its_channel();
DROP FUNCTION activity_source_system(text, text, text, text);
DROP FUNCTION activity_source_channel(text, text);
DROP FUNCTION activity_source_producer(text);
ALTER TABLE activity DROP CONSTRAINT activity_source_check;
