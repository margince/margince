-- pgmigrate:no-transaction
-- activity_review_template.archived_at and linkedin_connection.provider_member_ref
-- are NULL on every row, so every row already meets the predicate of the
-- partial unique indexes on them. These keys hold the same rows without it, and
-- stay when the next migration drops the columns and the partial indexes.
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS activity_review_template_one_per_key;
CREATE UNIQUE INDEX CONCURRENTLY activity_review_template_one_per_key
    ON activity_review_template (key);
DROP INDEX CONCURRENTLY IF EXISTS activity_review_template_one_asked_per_outcome;
CREATE UNIQUE INDEX CONCURRENTLY activity_review_template_one_asked_per_outcome
    ON activity_review_template (outcome) WHERE active;
DROP INDEX CONCURRENTLY IF EXISTS uq_linkedin_connection_natural_key;
CREATE UNIQUE INDEX CONCURRENTLY uq_linkedin_connection_natural_key
    ON linkedin_connection (owner_user_id, normalized_name, normalized_company, connected_on) NULLS NOT DISTINCT;
RESET lock_timeout;
