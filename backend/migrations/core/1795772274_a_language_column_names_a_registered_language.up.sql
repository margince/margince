SET LOCAL lock_timeout = '3s';

-- The languages stored data may carry. Core seeds the three it ships a
-- catalogue for, and a composed extension adds the others at boot, so a new
-- language needs no core migration. Adding a code here does not translate the
-- UI: a language without a catalogue falls back to English.
CREATE TABLE IF NOT EXISTS language (
    code text NOT NULL,
    label text NOT NULL,
    source text NOT NULL,
    CONSTRAINT language_pkey PRIMARY KEY (code),
    CONSTRAINT language_code_shape CHECK (code ~ '^[a-z]{2}$'),
    CONSTRAINT language_source_check CHECK (source IN ('core', 'unit'))
);

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."language" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"label":120,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"label":120,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."language" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"label":120,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"label":120,"source":20000}');

INSERT INTO language (code, label, source) VALUES
    ('de', 'German', 'core'),
    ('en', 'English', 'core'),
    ('vi', 'Vietnamese', 'core')
ON CONFLICT (code) DO NOTHING;

-- The two lists of three codes become the lookup. The keys are NOT VALID: a
-- new bad value is refused at once, and rows already stored are reported by
-- the unknown_language_values data task and validated in a later file.
ALTER TABLE activity DROP CONSTRAINT IF EXISTS activity_language_check;
ALTER TABLE app_user DROP CONSTRAINT IF EXISTS app_user_locale_shipped;

ALTER TABLE activity
    ADD CONSTRAINT activity_language_fkey
        FOREIGN KEY (language) REFERENCES language(code) ON DELETE RESTRICT ON UPDATE RESTRICT NOT VALID;
ALTER TABLE app_user
    ADD CONSTRAINT app_user_locale_fkey
        FOREIGN KEY (locale) REFERENCES language(code) ON DELETE RESTRICT ON UPDATE RESTRICT NOT VALID;
ALTER TABLE confirm_token
    ADD CONSTRAINT confirm_token_question_locale_fkey
        FOREIGN KEY (question_locale) REFERENCES language(code) ON DELETE RESTRICT ON UPDATE RESTRICT NOT VALID;
ALTER TABLE consent_text_version
    ADD CONSTRAINT consent_text_version_locale_fkey
        FOREIGN KEY (locale) REFERENCES language(code) ON DELETE RESTRICT ON UPDATE RESTRICT NOT VALID;
ALTER TABLE team_weekly_review
    ADD CONSTRAINT team_weekly_review_summary_language_fkey
        FOREIGN KEY (summary_language) REFERENCES language(code) ON DELETE RESTRICT ON UPDATE RESTRICT NOT VALID;
ALTER TABLE legal_entity
    ADD CONSTRAINT legal_entity_default_language_fkey
        FOREIGN KEY (default_language) REFERENCES language(code) ON DELETE RESTRICT ON UPDATE RESTRICT NOT VALID;
ALTER TABLE offer
    ADD CONSTRAINT offer_language_fkey
        FOREIGN KEY (language) REFERENCES language(code) ON DELETE RESTRICT ON UPDATE RESTRICT NOT VALID;
ALTER TABLE product_translation
    ADD CONSTRAINT product_translation_language_fkey
        FOREIGN KEY (language) REFERENCES language(code) ON DELETE RESTRICT ON UPDATE RESTRICT NOT VALID;

-- The two locale columns hold a language and an optional region (de-DE,
-- de-AT). A template for each region stays a template of its own, so they
-- take a shape check and no key.
ALTER TABLE offer_template
    ADD CONSTRAINT offer_template_locale_shape
        CHECK (locale ~ '^[a-z]{2}(-[A-Z]{2})?$') NOT VALID;
ALTER TABLE lead_privacy_notice
    ADD CONSTRAINT lead_privacy_notice_locale_shape
        CHECK (locale IS NULL OR locale ~ '^[a-z]{2}(-[A-Z]{2})?$') NOT VALID;
