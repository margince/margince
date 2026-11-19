-- pgmigrate:no-transaction
SET lock_timeout = '3s';

-- The per-contact primary indexes, built without blocking writers under a
-- temporary name. The next migration swaps them in.
DROP INDEX CONCURRENTLY IF EXISTS uq_contact_email_primary_next;
CREATE UNIQUE INDEX CONCURRENTLY uq_contact_email_primary_next ON contact_email (contact_id) WHERE is_primary AND archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS uq_contact_phone_primary_next;
CREATE UNIQUE INDEX CONCURRENTLY uq_contact_phone_primary_next ON contact_phone (contact_id) WHERE is_primary AND archived_at IS NULL;
