SET LOCAL lock_timeout = '3s';

DROP TRIGGER signal_source_channel ON signal;
DROP FUNCTION move_signal_source_onto_its_channel();
DROP FUNCTION signal_source_channel(text, text);
DROP FUNCTION signal_source_producer(text, text);
ALTER TABLE signal DROP CONSTRAINT signal_source_check;
ALTER TABLE signal DROP COLUMN source_system;
