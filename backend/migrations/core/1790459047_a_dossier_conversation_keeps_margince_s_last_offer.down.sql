SET LOCAL lock_timeout = '3s';

ALTER TABLE site_read DROP COLUMN IF EXISTS conversation_offer;
