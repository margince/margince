-- Each cascade from an activity or a connection finds its personal-channel rows
-- by index, and an outbound push finds the reply a client acked as sent.
SET LOCAL lock_timeout = '3s';
CREATE INDEX personal_channel_reply_activity_idx ON personal_channel_reply (activity_id);
CREATE INDEX personal_channel_reply_sent_idx ON personal_channel_reply (connection_id, network, provider_message_id)
    WHERE provider_message_id IS NOT NULL;
CREATE INDEX personal_channel_upload_connection_idx ON personal_channel_upload (connection_id);
CREATE INDEX personal_channel_message_connection_idx ON personal_channel_message (connection_id);
