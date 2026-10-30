-- pgmigrate:no-transaction
-- Drops the LinkedIn profile index, concurrently and safe to run again.
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_social_linkedin_profile;
