SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_participant VALIDATE CONSTRAINT fk_participant_domain_evidence_version;
