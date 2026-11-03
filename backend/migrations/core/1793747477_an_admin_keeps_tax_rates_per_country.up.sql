-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The tax rates the installation charges, by country and category, each from
-- the day it takes effect, and the category each product falls in.
--
-- A rate is added and never changed or removed, like an FX rate: an offer
-- sent under it keeps the rate it was sent with, and the sheet is the record
-- of what was in force when. "Exempt" is a category with no rate row. A
-- product with no category is not classified yet and prices as it always has.
SET LOCAL lock_timeout = '3s';

CREATE TABLE tax_rate (
    id uuid DEFAULT uuidv7() NOT NULL,
    -- Lower-case ISO 3166-1 alpha-2, the spelling installation.country uses.
    country text NOT NULL,
    category text NOT NULL,
    rate numeric(5,2) NOT NULL,
    valid_from date NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT tax_rate_pkey PRIMARY KEY (id),
    CONSTRAINT tax_rate_country_shape CHECK (country ~ '^[a-z]{2}$'),
    CONSTRAINT tax_rate_category_check CHECK (category IN ('standard', 'reduced')),
    CONSTRAINT tax_rate_rate_range CHECK (rate >= 0 AND rate < 100),
    CONSTRAINT tax_rate_once_per_day UNIQUE (country, category, valid_from)
);

ALTER TABLE product
    ADD COLUMN tax_category text,
    ADD CONSTRAINT product_tax_category_check
        CHECK (tax_category IS NULL OR tax_category IN ('standard', 'reduced', 'exempt')) NOT VALID;

-- Admin and ops keep the sheet, as they keep the FX rates; no system role
-- changes or removes a rate. Every other system role holds nothing. Presence-
-- guarded: a hand-set grant stays.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,tax_rate}',
        CASE WHEN key IN ('admin', 'ops')
             THEN '{"create":true,"read":true,"update":false,"delete":false}'::jsonb
             ELSE '{"create":false,"read":false,"update":false,"delete":false}'::jsonb
        END, true)
    WHERE is_system
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'tax_rate';

-- The sheet only grows. Last in this file, so no statement above runs under it.
CREATE FUNCTION tax_rate_refuse_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'a tax rate is added and never changed or removed; record a new rate from a later day'
        USING ERRCODE = 'check_violation';
END;
$$;

CREATE TRIGGER trg_tax_rate_append_only
    BEFORE UPDATE OR DELETE ON tax_rate
    FOR EACH ROW EXECUTE FUNCTION tax_rate_refuse_change();
