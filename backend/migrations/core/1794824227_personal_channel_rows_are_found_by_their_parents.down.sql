SET LOCAL lock_timeout = '3s';
DROP INDEX IF EXISTS personal_channel_message_connection_idx;
DROP INDEX IF EXISTS personal_channel_upload_connection_idx;
DROP INDEX IF EXISTS personal_channel_reply_sent_idx;
DROP INDEX IF EXISTS personal_channel_reply_activity_idx;
