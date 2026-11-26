SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS uq_app_user_chat_zalo_code, uq_app_user_chat_telegram_code, uq_app_user_chat_whatsapp_code,
    uq_app_user_chat_zalo, uq_app_user_chat_telegram, uq_app_user_chat_whatsapp;
ALTER TABLE app_user
    DROP CONSTRAINT IF EXISTS app_user_chat_codes_expire,
    DROP CONSTRAINT IF EXISTS app_user_chat_zalo_is_bounded,
    DROP CONSTRAINT IF EXISTS app_user_chat_zalo_is_verified,
    DROP CONSTRAINT IF EXISTS app_user_chat_telegram_is_an_id,
    DROP CONSTRAINT IF EXISTS app_user_chat_telegram_is_verified,
    DROP CONSTRAINT IF EXISTS app_user_chat_whatsapp_is_digits,
    DROP CONSTRAINT IF EXISTS app_user_chat_whatsapp_is_verified,
    DROP COLUMN IF EXISTS chat_zalo_code_expires_at, DROP COLUMN IF EXISTS chat_zalo_code_hash,
    DROP COLUMN IF EXISTS chat_telegram_code_expires_at, DROP COLUMN IF EXISTS chat_telegram_code_hash,
    DROP COLUMN IF EXISTS chat_whatsapp_code_expires_at, DROP COLUMN IF EXISTS chat_whatsapp_code_hash,
    DROP COLUMN IF EXISTS chat_zalo_verified_at, DROP COLUMN IF EXISTS chat_zalo_oa_id, DROP COLUMN IF EXISTS chat_zalo_user_id,
    DROP COLUMN IF EXISTS chat_telegram_verified_at, DROP COLUMN IF EXISTS chat_telegram_user_id,
    DROP COLUMN IF EXISTS chat_whatsapp_verified_at, DROP COLUMN IF EXISTS chat_whatsapp_phone;
