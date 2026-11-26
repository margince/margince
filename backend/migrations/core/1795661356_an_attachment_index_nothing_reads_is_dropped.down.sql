-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS attachment_account_ix;
CREATE INDEX CONCURRENTLY attachment_account_ix ON attachment USING btree (company_id, pinned DESC, created_at DESC) WHERE (archived_at IS NULL);
