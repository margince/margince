SET LOCAL lock_timeout = '3s';

DROP TRIGGER IF EXISTS stamp_participant_domain_evidence ON activity_participant;
ALTER TABLE activity_participant DROP CONSTRAINT IF EXISTS fk_participant_domain_evidence_version;
