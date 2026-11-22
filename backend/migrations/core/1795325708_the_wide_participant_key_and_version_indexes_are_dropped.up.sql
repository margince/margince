-- pgmigrate:no-transaction
SET lock_timeout = '2s';
-- The digest key holds participant identity, and a trigger copies the evidence the two version indexes served.
DROP INDEX CONCURRENTLY IF EXISTS uq_activity_participant;
DROP INDEX CONCURRENTLY IF EXISTS idx_participant_domain_evidence_parent;
DROP INDEX CONCURRENTLY IF EXISTS uq_activity_domain_evidence_version;
RESET lock_timeout;
