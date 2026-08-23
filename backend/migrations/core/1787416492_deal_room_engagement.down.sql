SET LOCAL lock_timeout = '5s';
DROP TABLE IF EXISTS deal_room_engagement;
ALTER TABLE deal_room_document DROP CONSTRAINT IF EXISTS uq_deal_room_document_in_room;
