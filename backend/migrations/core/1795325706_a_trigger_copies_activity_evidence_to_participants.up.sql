SET LOCAL lock_timeout = '3s';

-- A participant insert stamps under a key-share lock on its activity.
-- The row lock makes that insert wait for this change and then read the changed row.
-- Its name sorts last, so the lock follows the advisory locks of the triggers before it.
CREATE FUNCTION wait_for_participant_stamps() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 PERFORM 1 FROM activity WHERE id = OLD.id FOR UPDATE;
 RETURN NEW;
END;
$$;

CREATE TRIGGER wait_for_participant_stamps
 BEFORE UPDATE ON activity
 FOR EACH ROW WHEN (NEW.domain_evidence_version IS DISTINCT FROM OLD.domain_evidence_version)
 EXECUTE FUNCTION wait_for_participant_stamps();

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
