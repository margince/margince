-- Back to a repeated delivery landing twice.
SET LOCAL lock_timeout = '3s';

DROP INDEX uq_deal_room_comment_request;
ALTER TABLE deal_room_comment DROP COLUMN request_id;

DROP INDEX uq_deal_room_thread_request;
ALTER TABLE deal_room_thread DROP COLUMN request_id;
