SET LOCAL lock_timeout = '5s';

ALTER TABLE chat_link DROP COLUMN read_disabled, DROP COLUMN post_disabled, DROP COLUMN read_since;
