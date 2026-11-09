-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS uq_activity_domain_evidence_version;
CREATE UNIQUE INDEX CONCURRENTLY uq_activity_domain_evidence_version
 ON activity(id, domain_evidence_version);
