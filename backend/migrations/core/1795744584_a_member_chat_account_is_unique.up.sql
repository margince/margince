-- pgmigrate:no-transaction
-- One identity belongs to one member, and one code to one member. Built while
-- members are written.
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS uq_app_user_chat_whatsapp;
CREATE UNIQUE INDEX CONCURRENTLY uq_app_user_chat_whatsapp ON app_user (chat_whatsapp_phone) WHERE chat_whatsapp_phone IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS uq_app_user_chat_telegram;
CREATE UNIQUE INDEX CONCURRENTLY uq_app_user_chat_telegram ON app_user (chat_telegram_user_id) WHERE chat_telegram_user_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS uq_app_user_chat_zalo;
CREATE UNIQUE INDEX CONCURRENTLY uq_app_user_chat_zalo ON app_user (chat_zalo_oa_id, chat_zalo_user_id) WHERE chat_zalo_user_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS uq_app_user_chat_whatsapp_code;
CREATE UNIQUE INDEX CONCURRENTLY uq_app_user_chat_whatsapp_code ON app_user (chat_whatsapp_code_hash) WHERE chat_whatsapp_code_hash IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS uq_app_user_chat_telegram_code;
CREATE UNIQUE INDEX CONCURRENTLY uq_app_user_chat_telegram_code ON app_user (chat_telegram_code_hash) WHERE chat_telegram_code_hash IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS uq_app_user_chat_zalo_code;
CREATE UNIQUE INDEX CONCURRENTLY uq_app_user_chat_zalo_code ON app_user (chat_zalo_code_hash) WHERE chat_zalo_code_hash IS NOT NULL;
