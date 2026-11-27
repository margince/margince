-- The columns come back empty, with the checks they had.
SET LOCAL lock_timeout = '3s';

ALTER TABLE team_weekly_review_outlook
    ADD COLUMN best_case_minor bigint
        CONSTRAINT team_weekly_review_outlook_best_case_minor_js_safe
        CHECK (best_case_minor >= -9007199254740991 AND best_case_minor <= 9007199254740991);
ALTER TABLE weekly_review_outlook
    ADD COLUMN best_case_minor bigint
        CONSTRAINT weekly_review_outlook_best_case_minor_js_safe
        CHECK (best_case_minor >= -9007199254740991 AND best_case_minor <= 9007199254740991);
ALTER TABLE forecast_snapshot
    ADD COLUMN best_case_minor bigint
        CONSTRAINT forecast_snapshot_best_case_minor_js_safe
        CHECK (best_case_minor >= -9007199254740991 AND best_case_minor <= 9007199254740991),
    DROP CONSTRAINT forecast_snapshot_readings_not_negative,
    ADD CONSTRAINT forecast_snapshot_readings_not_negative CHECK (
        won_minor >= 0 AND evidence_minor >= 0 AND best_case_minor >= 0
        AND open_minor >= 0 AND weighted_minor >= 0) NOT VALID;
