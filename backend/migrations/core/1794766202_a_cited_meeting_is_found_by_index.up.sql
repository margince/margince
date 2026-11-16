-- pgmigrate:no-transaction
-- Deleting an activity clears the company fields that cite it. This index finds
-- them instead of scanning every field. The first drop clears an invalid index
-- a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_company_profile_field_source_activity;
CREATE INDEX CONCURRENTLY idx_company_profile_field_source_activity
    ON company_profile_field (source_activity_id)
    WHERE source_activity_id IS NOT NULL;
