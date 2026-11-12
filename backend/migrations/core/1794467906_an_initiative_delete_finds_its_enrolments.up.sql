-- pgmigrate:no-transaction
-- Deleting an initiative cascades into its dated enrolment rows, and the only
-- index that leads with initiative_id covers open rows alone. Built
-- concurrently. The drop clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS sales_initiative_enrolment_initiative_idx;
CREATE INDEX CONCURRENTLY sales_initiative_enrolment_initiative_idx
    ON sales_initiative_enrolment (initiative_id);
