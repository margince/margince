-- pgmigrate:no-transaction
-- Drops the index a folded seat login is found by.
DROP INDEX CONCURRENTLY IF EXISTS idx_app_user_login_folded;
