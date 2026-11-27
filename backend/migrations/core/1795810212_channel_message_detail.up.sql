SET LOCAL lock_timeout = '5s';

ALTER TABLE chat_link ADD COLUMN message_detail text CHECK (message_detail IN ('brief','full'));
ALTER TABLE chat_post_item ADD COLUMN message_detail text CHECK (message_detail IN ('legacy','brief','full'));
