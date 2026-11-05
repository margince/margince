-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS privacy_notice_case_review_queue;
CREATE INDEX CONCURRENTLY privacy_notice_case_review_queue ON privacy_notice_case (coalesce(due_at, created_at), id);
