SET LOCAL lock_timeout = '3s';

-- A member's own account on a chat network, proved by sending a one-time code
-- from it to a company account. A value is stored only once it is verified, so
-- a value and its verified_at are always set together, and an unverified value
-- cannot exist to be used for recognition. A Zalo user id is per official
-- account, so one Zalo field covers one OA and names it.
ALTER TABLE app_user
    ADD COLUMN chat_whatsapp_phone text,
    ADD COLUMN chat_whatsapp_verified_at timestamptz,
    ADD COLUMN chat_telegram_user_id text,
    ADD COLUMN chat_telegram_verified_at timestamptz,
    ADD COLUMN chat_zalo_user_id text,
    ADD COLUMN chat_zalo_oa_id text,
    ADD COLUMN chat_zalo_verified_at timestamptz,
    ADD COLUMN chat_whatsapp_code_hash text,
    ADD COLUMN chat_whatsapp_code_expires_at timestamptz,
    ADD COLUMN chat_telegram_code_hash text,
    ADD COLUMN chat_telegram_code_expires_at timestamptz,
    ADD COLUMN chat_zalo_code_hash text,
    ADD COLUMN chat_zalo_code_expires_at timestamptz,
    ADD CONSTRAINT app_user_chat_whatsapp_is_verified CHECK ((chat_whatsapp_phone IS NULL) = (chat_whatsapp_verified_at IS NULL)),
    ADD CONSTRAINT app_user_chat_whatsapp_is_digits CHECK (chat_whatsapp_phone IS NULL OR chat_whatsapp_phone ~ '^[0-9]{5,20}$'),
    ADD CONSTRAINT app_user_chat_telegram_is_verified CHECK ((chat_telegram_user_id IS NULL) = (chat_telegram_verified_at IS NULL)),
    ADD CONSTRAINT app_user_chat_telegram_is_an_id CHECK (chat_telegram_user_id IS NULL OR chat_telegram_user_id ~ '^[0-9]{1,20}$'),
    ADD CONSTRAINT app_user_chat_zalo_is_verified CHECK (
        (chat_zalo_user_id IS NULL) = (chat_zalo_verified_at IS NULL) AND (chat_zalo_user_id IS NULL) = (chat_zalo_oa_id IS NULL)),
    ADD CONSTRAINT app_user_chat_zalo_is_bounded CHECK (chat_zalo_user_id IS NULL OR (char_length(chat_zalo_user_id) <= 64 AND char_length(chat_zalo_oa_id) <= 64)),
    ADD CONSTRAINT app_user_chat_codes_expire CHECK (
        (chat_whatsapp_code_hash IS NULL) = (chat_whatsapp_code_expires_at IS NULL)
        AND (chat_telegram_code_hash IS NULL) = (chat_telegram_code_expires_at IS NULL)
        AND (chat_zalo_code_hash IS NULL) = (chat_zalo_code_expires_at IS NULL));

-- One identity belongs to one member.
CREATE UNIQUE INDEX uq_app_user_chat_whatsapp ON app_user (chat_whatsapp_phone) WHERE chat_whatsapp_phone IS NOT NULL;
CREATE UNIQUE INDEX uq_app_user_chat_telegram ON app_user (chat_telegram_user_id) WHERE chat_telegram_user_id IS NOT NULL;
CREATE UNIQUE INDEX uq_app_user_chat_zalo ON app_user (chat_zalo_oa_id, chat_zalo_user_id) WHERE chat_zalo_user_id IS NOT NULL;
-- A code names one pending verification, so two members cannot hold the same hash.
CREATE UNIQUE INDEX uq_app_user_chat_whatsapp_code ON app_user (chat_whatsapp_code_hash) WHERE chat_whatsapp_code_hash IS NOT NULL;
CREATE UNIQUE INDEX uq_app_user_chat_telegram_code ON app_user (chat_telegram_code_hash) WHERE chat_telegram_code_hash IS NOT NULL;
CREATE UNIQUE INDEX uq_app_user_chat_zalo_code ON app_user (chat_zalo_code_hash) WHERE chat_zalo_code_hash IS NOT NULL;
