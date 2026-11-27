SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user VALIDATE CONSTRAINT app_user_chat_whatsapp_is_verified;
ALTER TABLE app_user VALIDATE CONSTRAINT app_user_chat_whatsapp_is_digits;
ALTER TABLE app_user VALIDATE CONSTRAINT app_user_chat_telegram_is_verified;
ALTER TABLE app_user VALIDATE CONSTRAINT app_user_chat_telegram_is_an_id;
ALTER TABLE app_user VALIDATE CONSTRAINT app_user_chat_zalo_is_verified;
ALTER TABLE app_user VALIDATE CONSTRAINT app_user_chat_zalo_is_bounded;
ALTER TABLE app_user VALIDATE CONSTRAINT app_user_chat_codes_expire;
