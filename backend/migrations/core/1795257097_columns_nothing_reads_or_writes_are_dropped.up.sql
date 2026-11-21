-- No statement writes or reads these columns. Each drop takes its own check.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_contribution
    DROP COLUMN in_best_case,
    DROP COLUMN qualification_complete,
    DROP COLUMN viability_assessed_at,
    DROP COLUMN qualified_policy_version,
    DROP COLUMN viability_disposition,
    DROP COLUMN category;
ALTER TABLE assurance_exception DROP COLUMN evidence_refs, DROP COLUMN version;
ALTER TABLE finance_connection DROP COLUMN capabilities, DROP COLUMN sync_cursor;
ALTER TABLE assurance_run DROP COLUMN version;
ALTER TABLE assurance_source_coverage DROP COLUMN version;
ALTER TABLE assurance_task_item DROP COLUMN state;
ALTER TABLE capture_sync_state DROP COLUMN backfill_active;
ALTER TABLE close_date_run DROP COLUMN version;
ALTER TABLE workflow_run DROP COLUMN error;
