-- pgmigrate:no-transaction
SET lock_timeout = '5s';
-- A unit's proposal and a colleague's question are keyed apart, so one never
-- takes the place of the other for the same message and target. The drops
-- before each build repair a failed one.
--
-- The old key goes in this same release, not the next: kept, it would collide
-- with a question row and fail capture's insert. While a rolling update runs,
-- an older server's extension proposal names the old key and fails until that
-- server is replaced.
DROP INDEX CONCURRENTLY IF EXISTS idx_activity_link_suggestion_for_user;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_activity_link_suggestion_for_user
    ON activity_link_suggestion (for_user) WHERE for_user IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS uq_activity_link_suggestion_unit_target;
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_activity_link_suggestion_unit_target
    ON activity_link_suggestion (activity_id, entity_type, entity_id) WHERE unit IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS uq_activity_link_suggestion_question;
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_activity_link_suggestion_question
    ON activity_link_suggestion (activity_id, entity_type, entity_id, for_user) WHERE for_user IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS uq_activity_link_suggestion_target;
RESET lock_timeout;
