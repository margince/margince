-- pgmigrate:no-transaction
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS uq_app_user_chat_whatsapp;
DROP INDEX CONCURRENTLY IF EXISTS uq_app_user_chat_telegram;
DROP INDEX CONCURRENTLY IF EXISTS uq_app_user_chat_zalo;
DROP INDEX CONCURRENTLY IF EXISTS uq_app_user_chat_whatsapp_code;
DROP INDEX CONCURRENTLY IF EXISTS uq_app_user_chat_telegram_code;
DROP INDEX CONCURRENTLY IF EXISTS uq_app_user_chat_zalo_code;
