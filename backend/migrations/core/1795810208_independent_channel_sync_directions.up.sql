SET LOCAL lock_timeout = '5s';

ALTER TABLE chat_link ADD COLUMN read_disabled boolean NOT NULL DEFAULT false;
ALTER TABLE chat_link ADD COLUMN post_disabled boolean NOT NULL DEFAULT false;
ALTER TABLE chat_link ADD COLUMN read_since timestamptz;
