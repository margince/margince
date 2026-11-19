-- pgmigrate:no-transaction
SET lock_timeout = '3s';

DROP INDEX CONCURRENTLY IF EXISTS uq_contact_phone_primary_next;
DROP INDEX CONCURRENTLY IF EXISTS uq_contact_email_primary_next;
