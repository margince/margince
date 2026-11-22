SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_participant ADD CONSTRAINT fk_participant_domain_evidence_version
 FOREIGN KEY (activity_id, domain_evidence_parent_version)
 REFERENCES activity(id, domain_evidence_version)
 ON UPDATE CASCADE ON DELETE CASCADE NOT VALID;

DROP TRIGGER copy_activity_domain_evidence ON activity;
DROP FUNCTION copy_activity_domain_evidence();

CREATE OR REPLACE FUNCTION advance_activity_domain_evidence_version() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.occurred_at, NEW.occurred_precision, NEW.archived_at, NEW.restricted_at)
 IS DISTINCT FROM ROW(OLD.occurred_at, OLD.occurred_precision, OLD.archived_at, OLD.restricted_at) THEN
  NEW.domain_evidence_version := OLD.domain_evidence_version + 1;
 END IF;
 RETURN NEW;
END;
$$;
