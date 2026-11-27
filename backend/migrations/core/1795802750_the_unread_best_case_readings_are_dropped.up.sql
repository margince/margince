-- No statement reads these columns; only the review template's insert wrote
-- its two, and its audit row names who added it. The floor on the snapshot's
-- readings named best_case_minor, so it is rebuilt without it and validated
-- by the next file.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_snapshot
    DROP CONSTRAINT forecast_snapshot_readings_not_negative,
    ADD CONSTRAINT forecast_snapshot_readings_not_negative CHECK (
        won_minor >= 0 AND evidence_minor >= 0
        AND open_minor >= 0 AND weighted_minor >= 0) NOT VALID,
    DROP COLUMN best_case_minor;
ALTER TABLE weekly_review_outlook DROP COLUMN best_case_minor;
ALTER TABLE team_weekly_review_outlook DROP COLUMN best_case_minor;
ALTER TABLE finance_customer_link DROP COLUMN source_updated_at;
ALTER TABLE activity_review_template DROP COLUMN source, DROP COLUMN captured_by;
