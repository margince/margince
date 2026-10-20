-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The legal entities this installation trades as, and which entity each user
-- works for.
--
-- Every installation gets exactly one default entity here. It stores none of
-- the fields the anchor company already holds: its name, legal name, address,
-- VAT ID, register entry and logo are read from the anchor, so the company form
-- stays their only editor. Every further entity stores its own.
--
-- A user with no legal_entity_member row works for the default entity.
SET LOCAL lock_timeout = '3s';

CREATE TABLE legal_entity (
    id uuid DEFAULT uuidv7() NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    display_name text,
    legal_name text,
    address_line1 text,
    address_line2 text,
    address_postal_code text,
    address_city text,
    address_region text,
    vat_id text,
    register_court text,
    register_number text,
    logo_object_key text,
    -- Lower-case ISO 3166-1 alpha-2, the spelling installation.country uses.
    -- NULL on the default entity means the installation's country.
    country text,
    -- ISO 639-1. NULL means the base text.
    default_language text,
    bank_account_holder text,
    iban text,
    bic text,
    imprint text,
    captured_by text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    archived_at timestamptz,

    CONSTRAINT legal_entity_pkey PRIMARY KEY (id),
    CONSTRAINT legal_entity_default_reads_the_anchor CHECK (NOT is_default OR (
        display_name IS NULL AND legal_name IS NULL
        AND address_line1 IS NULL AND address_line2 IS NULL AND address_postal_code IS NULL
        AND address_city IS NULL AND address_region IS NULL
        AND vat_id IS NULL AND register_court IS NULL AND register_number IS NULL
        AND logo_object_key IS NULL)),
    CONSTRAINT legal_entity_names_itself CHECK (is_default OR (
        btrim(coalesce(display_name, '')) <> ''
        AND btrim(coalesce(legal_name, '')) <> ''
        AND country IS NOT NULL)),
    CONSTRAINT legal_entity_default_is_permanent CHECK (NOT is_default OR archived_at IS NULL),
    CONSTRAINT legal_entity_country_shape CHECK (country IS NULL OR country ~ '^[a-z]{2}$'),
    CONSTRAINT legal_entity_language_shape CHECK (default_language IS NULL OR default_language ~ '^[a-z]{2}$'),
    CONSTRAINT legal_entity_iban_shape CHECK (iban IS NULL OR iban ~ '^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$'),
    CONSTRAINT legal_entity_bic_shape CHECK (bic IS NULL OR bic ~ '^[A-Z]{6}[A-Z0-9]{2}([A-Z0-9]{3})?$'),
    CONSTRAINT legal_entity_imprint_bounded CHECK (imprint IS NULL OR length(imprint) <= 4000),
    CONSTRAINT legal_entity_version_js_safe
        CHECK (version >= '-9007199254740991'::bigint AND version <= '9007199254740991'::bigint)
);

-- One default entity per installation. It can never be archived, so the index
-- needs no archive predicate.
CREATE UNIQUE INDEX uq_legal_entity_default ON legal_entity ((true)) WHERE is_default;
-- Two live entities with one name would be two rows a picker cannot tell apart.
CREATE UNIQUE INDEX uq_legal_entity_live_name ON legal_entity (lower(display_name))
    WHERE archived_at IS NULL AND NOT is_default;

CREATE TRIGGER trg_legal_entity_updated
    BEFORE UPDATE ON legal_entity
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();

-- The CHECK keeps the default live. This keeps it the default and keeps it in
-- the table, which a CHECK cannot see.
CREATE FUNCTION legal_entity_refuse_default_retirement() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF TG_OP = 'DELETE' AND OLD.is_default THEN
    RAISE EXCEPTION 'legal entity % is the default entity and may not be deleted', OLD.id
      USING ERRCODE = 'check_violation',
            CONSTRAINT = 'legal_entity_default_is_permanent';
  END IF;
  IF TG_OP = 'UPDATE' AND OLD.is_default AND NOT NEW.is_default THEN
    RAISE EXCEPTION 'legal entity % is the default entity and may not be demoted', OLD.id
      USING ERRCODE = 'check_violation',
            CONSTRAINT = 'legal_entity_default_is_permanent';
  END IF;
  IF TG_OP = 'DELETE' THEN
    RETURN OLD;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER trg_legal_entity_default_is_permanent
    BEFORE UPDATE OR DELETE ON legal_entity
    FOR EACH ROW EXECUTE FUNCTION legal_entity_refuse_default_retirement();

-- user_id is the key, so a user works for one entity at a time. assigned_by is
-- the principal stamp, as captured_by is everywhere else.
CREATE TABLE legal_entity_member (
    user_id uuid NOT NULL,
    legal_entity_id uuid NOT NULL,
    assigned_at timestamptz DEFAULT now() NOT NULL,
    assigned_by text NOT NULL,

    CONSTRAINT legal_entity_member_pkey PRIMARY KEY (user_id),
    CONSTRAINT legal_entity_member_user_id_fkey FOREIGN KEY (user_id)
        REFERENCES app_user(id) ON DELETE CASCADE,
    -- RESTRICT: an entity is archived, never deleted, and only once nobody works for it.
    CONSTRAINT legal_entity_member_legal_entity_id_fkey FOREIGN KEY (legal_entity_id)
        REFERENCES legal_entity(id) ON DELETE RESTRICT
);

CREATE INDEX idx_legal_entity_member_entity ON legal_entity_member (legal_entity_id);

WITH inserted AS (
  INSERT INTO legal_entity (is_default, captured_by)
  VALUES (true, 'system:migration')
  RETURNING id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'create', 'legal_entity', inserted.id,
       NULL, jsonb_build_object('is_default', true)
  FROM inserted;

-- Admin describes and archives the entities; every other system role reads
-- them, because every seat's offers and pages will carry one. Presence-guarded:
-- a hand-set grant stays.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,legal_entity}',
        '{"create":true,"read":true,"update":true,"delete":true}'::jsonb, true)
    WHERE is_system
      AND key = 'admin'
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'legal_entity';

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,legal_entity}',
        '{"create":false,"read":true,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'legal_entity';
