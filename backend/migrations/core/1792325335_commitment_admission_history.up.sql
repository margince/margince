SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_import ADD COLUMN source_occurred_at timestamptz;

CREATE TABLE activity_commitment_admission (
    source_activity_id uuid NOT NULL REFERENCES activity(id) ON DELETE RESTRICT,
    locator text NOT NULL,
    summary text NOT NULL,
    source_quote text NOT NULL,
    outcome text NOT NULL,
    current_work boolean NOT NULL,
    rule_version text NOT NULL,
    evidence_ids uuid[] NOT NULL,
    evaluated_at timestamptz NOT NULL,
    renewed_by uuid REFERENCES activity(id) ON DELETE RESTRICT,
    renewed_at timestamptz,
    captured_by text NOT NULL,
    CHECK ((renewed_by IS NULL) = (renewed_at IS NULL)),
    PRIMARY KEY (source_activity_id, locator)
);
