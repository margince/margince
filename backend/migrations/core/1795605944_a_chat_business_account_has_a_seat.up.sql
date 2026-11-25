-- 1795605944: a_chat_business_account_has_a_seat.
SET LOCAL lock_timeout = '3s';

-- A business chat account (Telegram Business, WhatsApp Business, Zalo OA) files
-- its messages through a seat in capture_connection, the table that already holds
-- what a mailbox asks of its mail. A company's account is a team seat like a
-- shared mailbox (no user_id, a reader team, posture held or shared); a member's
-- own account is a member seat (user_id, posture classified by default). The
-- link row of the account names its seat.
ALTER TABLE channel_connection ADD COLUMN IF NOT EXISTS seat_id uuid
    REFERENCES capture_connection(id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX IF NOT EXISTS uq_channel_connection_seat ON channel_connection (seat_id)
    WHERE seat_id IS NOT NULL AND archived_at IS NULL;

ALTER TABLE capture_connection DROP CONSTRAINT IF EXISTS capture_connection_provider_check;
ALTER TABLE capture_connection ADD CONSTRAINT capture_connection_provider_check
    CHECK (provider IN ('gmail', 'gcal', 'gdocs', 'imap', 'graph', 'graphcal', 'whatsapp', 'telegram',
                        'offline_demo', 'test_mailbox', 'telegram_business', 'zalo_oa')) NOT VALID;
