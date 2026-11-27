-- pgmigrate:no-transaction
SET lock_timeout = '5s';
DROP INDEX CONCURRENTLY IF EXISTS uq_activity_link_suggestion_target;
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_activity_link_suggestion_target
    ON activity_link_suggestion (activity_id, entity_type, entity_id);
DROP INDEX CONCURRENTLY IF EXISTS uq_activity_link_suggestion_question;
DROP INDEX CONCURRENTLY IF EXISTS uq_activity_link_suggestion_unit_target;
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_link_suggestion_for_user;
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_link_suggestion_activity;
RESET lock_timeout;
