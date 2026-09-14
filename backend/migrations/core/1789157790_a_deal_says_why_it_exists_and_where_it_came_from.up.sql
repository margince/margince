SET LOCAL lock_timeout = '3s';

-- A deal carries four human answers it could not hold before: what the customer
-- needs in a person's own words, why the deal exists commercially, how much a
-- human says it matters, and which business channel brought it.
--
-- acquisition_source is deliberately NOT the existing `source` column beside it.
-- `source` records how the RECORD reached Margince — a connector, an import, a
-- crawl — and the lead vocabulary that administers it also carries scoring
-- intent. This records how the OPPORTUNITY reached the business. A deal typed in
-- by hand can be a referral and an imported one can be outbound, so one column
-- cannot answer both without lying about one of them.

CREATE TABLE deal_acquisition_source (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    key         text NOT NULL UNIQUE,
    label       text NOT NULL,
    sort_order  integer NOT NULL DEFAULT 0,
    active      boolean NOT NULL DEFAULT true,
    -- Seeded with the installation. Every field stays editable; `system` only
    -- means the row is part of the shipped vocabulary.
    system      boolean NOT NULL DEFAULT false,
    version     bigint NOT NULL DEFAULT 1,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT deal_acquisition_source_key_shape
        CHECK (key = lower(key) AND length(btrim(key)) > 0),
    CONSTRAINT deal_acquisition_source_label_present
        CHECK (length(btrim(label)) > 0)
);

CREATE TRIGGER trg_deal_acquisition_source_updated
    BEFORE UPDATE ON deal_acquisition_source
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();


INSERT INTO deal_acquisition_source (key, label, sort_order, system) VALUES
    ('inbound',           'Inbound',           10, true),
    ('outbound',          'Outbound',          20, true),
    ('referral',          'Referral',          30, true),
    ('partner',           'Partner',           40, true),
    ('event',             'Event',             50, true),
    ('existing_customer', 'Existing customer', 60, true),
    ('employee_referral', 'Employee referral', 70, true),
    ('other',             'Other',             80, true);

ALTER TABLE deal
    ADD COLUMN description        text,
    ADD COLUMN commercial_motion  text,
    ADD COLUMN priority           text,
    ADD COLUMN acquisition_source text;

ALTER TABLE deal
    ADD CONSTRAINT deal_commercial_motion_check
        CHECK (commercial_motion IS NULL OR commercial_motion IN
            ('new_business', 'renewal', 'upsell', 'cross_sell', 'expansion', 'existing_business')),
    ADD CONSTRAINT deal_priority_check
        CHECK (priority IS NULL OR priority IN ('low', 'medium', 'high')),
    ADD CONSTRAINT deal_description_bounded
        CHECK (description IS NULL OR length(description) <= 20000),
    ADD CONSTRAINT deal_acquisition_source_fkey
        FOREIGN KEY (acquisition_source) REFERENCES deal_acquisition_source (key)
        ON UPDATE CASCADE ON DELETE RESTRICT;

-- Priority sorts High → Medium → Low → unset, which is its business order and
-- not its alphabetical one ('high' < 'low' < 'medium' would interleave them
-- meaninglessly). The list machinery renders ORDER BY as ONE quoted column
-- identifier, so the rank has to BE a column rather than a CASE expression at
-- the call site. Generated and stored, so it cannot drift from the text beside
-- it: there is no writer that could set one without the other.
ALTER TABLE deal
    ADD COLUMN priority_rank smallint
    GENERATED ALWAYS AS (CASE priority WHEN 'high' THEN 3 WHEN 'medium' THEN 2 WHEN 'low' THEN 1 END) STORED;

CREATE INDEX idx_deal_priority_rank ON deal (priority_rank DESC NULLS LAST)
    WHERE archived_at IS NULL;
CREATE INDEX idx_deal_commercial_motion ON deal (commercial_motion)
    WHERE archived_at IS NULL AND commercial_motion IS NOT NULL;
CREATE INDEX idx_deal_acquisition_source ON deal (acquisition_source)
    WHERE archived_at IS NULL AND acquisition_source IS NOT NULL;

-- The brief joins the deal's search vector at weight B, under the name's A.
-- A person searching a phrase they wrote into a brief should find that deal,
-- but never above a deal whose NAME is what they typed. Postgres cannot alter a
-- generated expression in place, so the column is dropped and re-added with the
-- name half preserved exactly as the baseline spells it.
ALTER TABLE deal DROP COLUMN search_tsv;
ALTER TABLE deal ADD COLUMN search_tsv tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple'::regconfig, f_unaccent(COALESCE(name, ''::text))), 'A'::"char")
        || setweight(to_tsvector('simple'::regconfig, f_fold_apostrophes(COALESCE(name, ''::text))), 'A'::"char")
        || setweight(to_tsvector('simple'::regconfig, f_unaccent(COALESCE(description, ''::text))), 'B'::"char")
    ) STORED;

-- Dropping the column dropped idx_deal_search with it. Recreated verbatim:
-- without it every deal search falls back to a sequential scan, which is a
-- silent performance regression rather than a visible failure.
CREATE INDEX idx_deal_search ON deal USING gin (search_tsv);
