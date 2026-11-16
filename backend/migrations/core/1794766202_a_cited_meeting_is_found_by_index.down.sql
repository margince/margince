-- pgmigrate:no-transaction
-- Drops the index the fields citing an activity are found by.
DROP INDEX CONCURRENTLY IF EXISTS idx_company_profile_field_source_activity;
