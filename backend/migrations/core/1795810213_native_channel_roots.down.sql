SET LOCAL lock_timeout = '3s';
ALTER TABLE chat_post_item DROP COLUMN redacted_at;
DROP TABLE chat_channel_watch;
ALTER TABLE chat_post_item DROP COLUMN retry_at;
ALTER TABLE chat_post_item DROP COLUMN last_error;
DROP TABLE chat_team_thread;
DROP TABLE chat_team_installation;
DROP TABLE chat_bot_delivery;
DROP TABLE chat_root;
ALTER TABLE chat_link DROP CONSTRAINT chat_link_status_check;
ALTER TABLE chat_link ADD CONSTRAINT chat_link_status_check CHECK (status IN ('active','channel_gone')) NOT VALID;
ALTER TABLE chat_link DROP COLUMN root_id, DROP COLUMN card_paused, DROP COLUMN card_generation, DROP COLUMN native_identity, DROP COLUMN post_supported, DROP COLUMN include_history, DROP COLUMN native_cursor, DROP COLUMN native_scan_started_at, DROP COLUMN native_coverage_at, DROP COLUMN native_scan_error, DROP COLUMN last_post_error, DROP COLUMN native_refresh_at;
ALTER TABLE chat_link DROP CONSTRAINT chat_link_provider_check;
ALTER TABLE chat_link ADD CONSTRAINT chat_link_provider_check CHECK (provider='dispact') NOT VALID;
ALTER TABLE channel_connection DROP CONSTRAINT channel_connection_provider_check;
ALTER TABLE channel_connection ADD CONSTRAINT channel_connection_provider_check
 CHECK (provider IN ('telegram','telegram_business','dispact','whatsapp','zalo_oa')) NOT VALID;
