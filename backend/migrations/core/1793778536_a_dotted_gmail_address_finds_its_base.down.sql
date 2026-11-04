-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_email_base;
CREATE INDEX CONCURRENTLY idx_contact_email_base ON contact_email (regexp_replace(lower(email), '^([^+@]+)\+[^@]*@', '\1@')) WHERE archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_email_mailbox_base;
