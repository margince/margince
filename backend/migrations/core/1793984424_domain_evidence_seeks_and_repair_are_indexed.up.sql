-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_participant_domain_evidence_latest;
CREATE INDEX CONCURRENTLY idx_participant_domain_evidence_latest
 ON activity_participant USING gist
 ((reverse(address_domain_folded) COLLATE "C"), domain_evidence_precision, domain_evidence_at)
 WHERE domain_evidence_visible AND NOT past_party_cap AND address_domain_folded IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_participant_domain_evidence_incomplete;
CREATE INDEX CONCURRENTLY idx_participant_domain_evidence_incomplete
 ON activity_participant(id)
 WHERE domain_evidence_parent_version IS NULL OR domain_evidence_at IS NULL
 OR domain_evidence_precision IS NULL OR domain_evidence_visible IS NULL;

DROP INDEX CONCURRENTLY IF EXISTS idx_participant_domain_evidence_parent;
CREATE INDEX CONCURRENTLY idx_participant_domain_evidence_parent
 ON activity_participant(activity_id, domain_evidence_parent_version);
