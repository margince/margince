SET LOCAL lock_timeout = '3s';

ALTER TABLE scheduled_send
    DROP CONSTRAINT IF EXISTS scheduled_send_also_links_shape;

ALTER TABLE scheduled_send
    DROP COLUMN IF EXISTS also_links;
