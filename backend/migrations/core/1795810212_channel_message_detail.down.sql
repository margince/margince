SET LOCAL lock_timeout = '5s';

ALTER TABLE chat_post_item DROP COLUMN message_detail;
ALTER TABLE chat_link DROP COLUMN message_detail;
