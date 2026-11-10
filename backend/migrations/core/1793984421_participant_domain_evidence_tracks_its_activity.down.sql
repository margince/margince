SET LOCAL lock_timeout = '3s';

DROP TRIGGER IF EXISTS stamp_participant_domain_evidence ON activity_participant;
DROP FUNCTION IF EXISTS stamp_participant_domain_evidence();
DROP TRIGGER IF EXISTS advance_activity_domain_evidence_version ON activity;
DROP FUNCTION IF EXISTS advance_activity_domain_evidence_version();
ALTER TABLE activity_participant
 DROP COLUMN domain_evidence_visible,
 DROP COLUMN domain_evidence_precision,
 DROP COLUMN domain_evidence_at,
 DROP COLUMN domain_evidence_parent_version;
ALTER TABLE activity DROP COLUMN domain_evidence_version;
