SET LOCAL lock_timeout = '3s';

-- When each record joined and left a sales initiative. The initiatives module
-- writes it from the membership evaluation the members list reads, with every
-- rule match read rather than one bounded page. A row with no left_at is a
-- current membership; at most one per record.
CREATE TABLE sales_initiative_enrolment (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    initiative_id uuid NOT NULL REFERENCES sales_initiative(id) ON DELETE CASCADE,
    record_type text NOT NULL CONSTRAINT sales_initiative_enrolment_type_check CHECK (record_type IN ('lead', 'deal')),
    record_id uuid NOT NULL,
    reason text NOT NULL CONSTRAINT sales_initiative_enrolment_reason_check CHECK (reason IN ('rule', 'picked', 'carried')),
    joined_at timestamptz NOT NULL,
    left_at timestamptz,
    captured_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sales_initiative_enrolment_dates_check CHECK (left_at IS NULL OR left_at >= joined_at)
);

CREATE UNIQUE INDEX sales_initiative_enrolment_open_idx ON sales_initiative_enrolment (initiative_id, record_type, record_id) WHERE left_at IS NULL;
CREATE INDEX sales_initiative_enrolment_record_idx ON sales_initiative_enrolment (record_type, record_id);
