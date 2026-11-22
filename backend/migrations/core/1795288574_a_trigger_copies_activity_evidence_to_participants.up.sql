SET LOCAL lock_timeout = '3s';

-- A participant insert stamps under a key-share lock on its activity.
-- The row lock makes that insert wait for this change and then read the changed row.
CREATE OR REPLACE FUNCTION advance_activity_domain_evidence_version() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.occurred_at, NEW.occurred_precision, NEW.archived_at, NEW.restricted_at)
 IS DISTINCT FROM ROW(OLD.occurred_at, OLD.occurred_precision, OLD.archived_at, OLD.restricted_at) THEN
  PERFORM 1 FROM activity WHERE id = OLD.id FOR UPDATE;
  NEW.domain_evidence_version := OLD.domain_evidence_version + 1;
 END IF;
 RETURN NEW;
END;
$$;

CREATE FUNCTION copy_activity_domain_evidence() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 UPDATE activity_participant SET domain_evidence_parent_version = NEW.domain_evidence_version
  WHERE activity_id = NEW.id;
 RETURN NULL;
END;
$$;

CREATE TRIGGER copy_activity_domain_evidence
 AFTER UPDATE ON activity
 FOR EACH ROW WHEN (NEW.domain_evidence_version IS DISTINCT FROM OLD.domain_evidence_version)
 EXECUTE FUNCTION copy_activity_domain_evidence();

ALTER TABLE activity_participant DROP CONSTRAINT fk_participant_domain_evidence_version;
