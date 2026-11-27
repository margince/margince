-- pgmigrate:no-transaction
-- attachment_account_ix leads on attachment.company_id, which no writer sets,
-- so no read can use it. Dropped concurrently, so attachment writes never wait.
DROP INDEX CONCURRENTLY IF EXISTS attachment_account_ix;
