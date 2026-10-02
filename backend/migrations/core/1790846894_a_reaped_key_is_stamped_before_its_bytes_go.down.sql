SET LOCAL lock_timeout = '3s';

ALTER TABLE stored_object_intent DROP COLUMN reaping_since;
