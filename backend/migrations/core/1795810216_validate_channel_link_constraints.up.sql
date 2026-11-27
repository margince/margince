SET LOCAL lock_timeout = '3s';
ALTER TABLE chat_link VALIDATE CONSTRAINT chat_link_record_check;
ALTER TABLE chat_link VALIDATE CONSTRAINT chat_link_status_check;
ALTER TABLE chat_link VALIDATE CONSTRAINT chat_link_provider_check;
ALTER TABLE channel_connection VALIDATE CONSTRAINT channel_connection_provider_check;
