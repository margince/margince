SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_review_response DROP COLUMN revision;
ALTER TABLE communication_review DROP COLUMN kind;
ALTER TABLE saved_view DROP COLUMN shared_scope;
ALTER TABLE conversation_claim DROP COLUMN needs_review;
