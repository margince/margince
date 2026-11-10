-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS uq_activity_domain_evidence_version;
