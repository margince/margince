-- pgmigrate:no-transaction
SET lock_timeout = '2s';
DROP INDEX CONCURRENTLY IF EXISTS uq_activity_domain_evidence_version;
CREATE UNIQUE INDEX CONCURRENTLY uq_activity_domain_evidence_version
 ON activity(id, domain_evidence_version);
DROP INDEX CONCURRENTLY IF EXISTS idx_participant_domain_evidence_parent;
CREATE INDEX CONCURRENTLY idx_participant_domain_evidence_parent
 ON activity_participant(activity_id, domain_evidence_parent_version);
DROP INDEX CONCURRENTLY IF EXISTS uq_activity_participant;
CREATE UNIQUE INDEX CONCURRENTLY uq_activity_participant
 ON activity_participant (activity_id, role, user_id, contact_id, address, channel_user_id) NULLS NOT DISTINCT;
RESET lock_timeout;
