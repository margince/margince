SET LOCAL lock_timeout = '3s';

-- A lead answers two questions that lead.source used to answer as one. "Origin"
-- says where the lead came from (a referral, an event, a cold list). It is
-- chosen from this administered list by a human, an importer or capture, and
-- it is the only lead fact the score, the origin filter and the report read.
-- lead.source keeps meaning how the row reached Margince, and it never scores.
CREATE TABLE lead_origin (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    key         text NOT NULL UNIQUE,
    label       text NOT NULL,
    intent      text NOT NULL DEFAULT 'neutral',
    sort_order  integer NOT NULL DEFAULT 0,
    active      boolean NOT NULL DEFAULT true,
    system      boolean NOT NULL DEFAULT false,
    version     bigint NOT NULL DEFAULT 1,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT lead_origin_intent_check CHECK (intent IN ('high', 'neutral', 'low')),
    CONSTRAINT lead_origin_key_shape CHECK (key = lower(key) AND length(btrim(key)) > 0),
    CONSTRAINT lead_origin_label_present CHECK (length(btrim(label)) > 0),
    CONSTRAINT lead_origin_version_js_safe
        CHECK (version BETWEEN -9007199254740991 AND 9007199254740991)
);

INSERT INTO lead_origin (key, label, intent, sort_order, system) VALUES
    ('event',         'Event',         'neutral', 10, true),
    ('linkedin',      'LinkedIn',      'neutral', 20, true),
    ('referral',      'Referral',      'high',    30, true),
    ('partner',       'Partner',       'high',    40, true),
    ('website',       'Website',       'high',    50, true),
    ('cold_outreach', 'Cold outreach', 'low',     60, true),
    ('other',         'Other',         'neutral', 70, true);

-- Both columns are nullable and no existing row is rewritten, so a lead nobody
-- has classified scores neutral. The foreign key is added NOT VALID and
-- validated by the next migration, which keeps this one from scanning lead
-- while it holds the lock.
ALTER TABLE lead
    ADD COLUMN origin        text,
    ADD COLUMN origin_detail text;

ALTER TABLE lead
    ADD CONSTRAINT lead_origin_detail_bounded
        CHECK (origin_detail IS NULL OR length(origin_detail) <= 500) NOT VALID,
    ADD CONSTRAINT lead_origin_fkey
        FOREIGN KEY (origin) REFERENCES lead_origin (key)
        ON UPDATE CASCADE ON DELETE RESTRICT NOT VALID;

CREATE INDEX idx_lead_origin ON lead (origin)
    WHERE archived_at IS NULL AND origin IS NOT NULL;

-- Last, because a statement after CREATE TRIGGER reads as a rewrite of every table.
CREATE TRIGGER trg_lead_origin_updated
    BEFORE UPDATE ON lead_origin
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();
