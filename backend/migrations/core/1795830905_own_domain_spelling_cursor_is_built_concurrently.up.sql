-- pgmigrate:no-transaction
SET lock_timeout = '3s';

DROP INDEX CONCURRENTLY IF EXISTS idx_own_domain_spelling_cursor;
CREATE INDEX CONCURRENTLY idx_own_domain_spelling_cursor ON workspace_email_domain (identifier_spelling_row_id(domain));

RESET lock_timeout;
