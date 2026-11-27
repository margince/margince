-- pgmigrate:no-transaction
-- Runs after the columns are back: the partial keys and the purpose index
-- return, built concurrently, and the keys without a predicate go.
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS activity_review_template_live_key;
CREATE UNIQUE INDEX CONCURRENTLY activity_review_template_live_key
    ON activity_review_template (key) WHERE (archived_at IS NULL);
DROP INDEX CONCURRENTLY IF EXISTS activity_review_template_one_active_per_outcome;
CREATE UNIQUE INDEX CONCURRENTLY activity_review_template_one_active_per_outcome
    ON activity_review_template (outcome) WHERE (active AND (archived_at IS NULL));
DROP INDEX CONCURRENTLY IF EXISTS uq_linkedin_connection_natural;
CREATE UNIQUE INDEX CONCURRENTLY uq_linkedin_connection_natural
    ON linkedin_connection (owner_user_id, normalized_name, normalized_company, connected_on) NULLS NOT DISTINCT
    WHERE (provider_member_ref IS NULL);
DROP INDEX CONCURRENTLY IF EXISTS uq_linkedin_connection_provider;
CREATE UNIQUE INDEX CONCURRENTLY uq_linkedin_connection_provider
    ON linkedin_connection (owner_user_id, provider_member_ref) WHERE (provider_member_ref IS NOT NULL);
DROP INDEX CONCURRENTLY IF EXISTS idx_auth_token_user;
CREATE INDEX CONCURRENTLY idx_auth_token_user ON auth_token (user_id, purpose) WHERE (used_at IS NULL);
DROP INDEX CONCURRENTLY IF EXISTS uq_linkedin_connection_natural_key;
DROP INDEX CONCURRENTLY IF EXISTS activity_review_template_one_asked_per_outcome;
DROP INDEX CONCURRENTLY IF EXISTS activity_review_template_one_per_key;
RESET lock_timeout;
