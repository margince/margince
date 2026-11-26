SET LOCAL lock_timeout = '3s';

DELETE FROM channel_connection WHERE provider = 'telegram_business';

DELETE FROM capture_connection WHERE provider IN ('telegram_business', 'zalo_oa');
ALTER TABLE capture_connection DROP CONSTRAINT IF EXISTS capture_connection_provider_check;
ALTER TABLE capture_connection ADD CONSTRAINT capture_connection_provider_check
    CHECK (provider IN ('gmail', 'gcal', 'gdocs', 'imap', 'graph', 'graphcal', 'whatsapp', 'telegram',
                        'offline_demo', 'test_mailbox')) NOT VALID;

DROP INDEX IF EXISTS uq_channel_connection_seat;
ALTER TABLE channel_connection DROP COLUMN IF EXISTS seat_id;
