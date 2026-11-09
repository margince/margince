SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_participant DROP CONSTRAINT fk_participant_domain_evidence_version;
ALTER TABLE activity_participant ADD CONSTRAINT fk_participant_domain_evidence_version
 FOREIGN KEY (activity_id, domain_evidence_parent_version)
 REFERENCES activity(id, domain_evidence_version)
 ON UPDATE CASCADE ON DELETE CASCADE NOT VALID;
