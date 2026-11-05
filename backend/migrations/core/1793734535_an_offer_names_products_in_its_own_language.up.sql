-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A product can carry its name and description in further languages, and an
-- offer has a language its new lines take product text in. An offer written
-- before this file has none and keeps the products' own text.
--
-- The offer check is added NOT VALID and validated by the file beside this
-- one, so the offer table is not scanned under the ACCESS EXCLUSIVE lock.
SET LOCAL lock_timeout = '3s';

CREATE TABLE product_translation (
    product_id uuid NOT NULL,
    -- ISO 639-1, lower case, the spelling legal_entity.default_language uses.
    language text NOT NULL,
    name text NOT NULL,
    description text,
    captured_by text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT product_translation_pkey PRIMARY KEY (product_id, language),
    CONSTRAINT product_translation_product_id_fkey FOREIGN KEY (product_id)
        REFERENCES product(id) ON DELETE CASCADE,
    CONSTRAINT product_translation_language_shape CHECK (language ~ '^[a-z]{2}$'),
    CONSTRAINT product_translation_name_present CHECK (btrim(name) <> ''),
    CONSTRAINT product_translation_bounded CHECK (
        length(name) <= 500 AND (description IS NULL OR length(description) <= 4000)),
    CONSTRAINT product_translation_version_js_safe
        CHECK (version >= '-9007199254740991'::bigint AND version <= '9007199254740991'::bigint)
);

ALTER TABLE offer
    ADD COLUMN language text,
    ADD CONSTRAINT offer_language_shape CHECK (language IS NULL OR language ~ '^[a-z]{2}$') NOT VALID;

CREATE TRIGGER trg_product_translation_updated
    BEFORE UPDATE ON product_translation
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();
