SET LOCAL lock_timeout = '5s';

ALTER TABLE app_user
    DROP CONSTRAINT IF EXISTS app_user_delivery_hour_check,
    DROP CONSTRAINT IF EXISTS app_user_weekly_delivery_check,
    DROP CONSTRAINT IF EXISTS app_user_morning_brief_delivery_check,
    DROP COLUMN IF EXISTS delivery_hour_local,
    DROP COLUMN IF EXISTS quiet_day_notice,
    DROP COLUMN IF EXISTS weekly_delivery,
    DROP COLUMN IF EXISTS morning_brief_delivery;
