-- The columns come back empty, with the checks they had.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_contribution
    ADD COLUMN in_best_case boolean NOT NULL DEFAULT false,
    ADD COLUMN qualification_complete boolean,
    ADD COLUMN viability_assessed_at timestamptz,
    ADD COLUMN qualified_policy_version text,
    ADD COLUMN viability_disposition text
        CONSTRAINT forecast_contribution_viability_disposition_known CHECK (
            viability_disposition IS NULL
            OR viability_disposition IN ('none', 'open_review', 'override', 'unsupported')),
    ADD COLUMN category text;
ALTER TABLE assurance_exception
    ADD COLUMN evidence_refs jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN version bigint NOT NULL DEFAULT 1
        CONSTRAINT assurance_exception_version_js_safe
        CHECK (version >= -9007199254740991 AND version <= 9007199254740991);
ALTER TABLE finance_connection
    ADD COLUMN capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN sync_cursor text;
ALTER TABLE assurance_run
    ADD COLUMN version bigint NOT NULL DEFAULT 1
        CONSTRAINT assurance_run_version_js_safe
        CHECK (version >= -9007199254740991 AND version <= 9007199254740991);
ALTER TABLE assurance_source_coverage
    ADD COLUMN version bigint NOT NULL DEFAULT 1
        CONSTRAINT assurance_source_coverage_version_js_safe
        CHECK (version >= -9007199254740991 AND version <= 9007199254740991);
ALTER TABLE assurance_task_item
    ADD COLUMN state text NOT NULL DEFAULT 'open'
        CONSTRAINT assurance_task_item_state CHECK (state IN ('open', 'resolved', 'dismissed'));
ALTER TABLE capture_sync_state ADD COLUMN backfill_active boolean NOT NULL DEFAULT false;
ALTER TABLE close_date_run
    ADD COLUMN version bigint NOT NULL DEFAULT 1
        CONSTRAINT close_date_run_version_js_safe
        CHECK (version >= -9007199254740991 AND version <= 9007199254740991);
ALTER TABLE workflow_run ADD COLUMN error text;
