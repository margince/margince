SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_participant ADD CONSTRAINT fk_participant_domain_evidence_version
 FOREIGN KEY (activity_id, domain_evidence_parent_version)
 REFERENCES activity(id, domain_evidence_version)
 ON UPDATE CASCADE ON DELETE CASCADE NOT VALID;

DROP TRIGGER copy_activity_domain_evidence ON activity;
DROP FUNCTION copy_activity_domain_evidence();
DROP TRIGGER wait_for_participant_stamps ON activity;
DROP FUNCTION wait_for_participant_stamps();
