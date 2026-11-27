SET LOCAL lock_timeout = '3s';

ALTER TABLE lead_privacy_notice DROP CONSTRAINT IF EXISTS lead_privacy_notice_locale_shape;
ALTER TABLE offer_template DROP CONSTRAINT IF EXISTS offer_template_locale_shape;
ALTER TABLE product_translation DROP CONSTRAINT IF EXISTS product_translation_language_fkey;
ALTER TABLE offer DROP CONSTRAINT IF EXISTS offer_language_fkey;
ALTER TABLE legal_entity DROP CONSTRAINT IF EXISTS legal_entity_default_language_fkey;
ALTER TABLE team_weekly_review DROP CONSTRAINT IF EXISTS team_weekly_review_summary_language_fkey;
ALTER TABLE consent_text_version DROP CONSTRAINT IF EXISTS consent_text_version_locale_fkey;
ALTER TABLE confirm_token DROP CONSTRAINT IF EXISTS confirm_token_question_locale_fkey;
ALTER TABLE app_user DROP CONSTRAINT IF EXISTS app_user_locale_fkey;
ALTER TABLE activity DROP CONSTRAINT IF EXISTS activity_language_fkey;

ALTER TABLE app_user
    ADD CONSTRAINT app_user_locale_shipped CHECK (locale IN ('en', 'de', 'vi')) NOT VALID;
ALTER TABLE activity
    ADD CONSTRAINT activity_language_check CHECK (language IS NULL OR language IN ('de', 'en', 'vi')) NOT VALID;

DROP TABLE IF EXISTS language;
