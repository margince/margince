SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_participant ADD CONSTRAINT fk_participant_domain_evidence_version
 FOREIGN KEY (activity_id, domain_evidence_parent_version)
 REFERENCES activity(id, domain_evidence_version)
 ON UPDATE CASCADE ON DELETE CASCADE NOT VALID;

CREATE TRIGGER stamp_participant_domain_evidence
 BEFORE INSERT OR UPDATE ON activity_participant
 FOR EACH ROW EXECUTE FUNCTION stamp_participant_domain_evidence();
