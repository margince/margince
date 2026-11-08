SET LOCAL lock_timeout = '3s';

ALTER TABLE activity ADD COLUMN domain_evidence_version bigint NOT NULL DEFAULT 0;
ALTER TABLE activity_participant
 ADD COLUMN domain_evidence_parent_version bigint,
 ADD COLUMN domain_evidence_at timestamptz,
 ADD COLUMN domain_evidence_precision text,
 ADD COLUMN domain_evidence_visible boolean;

CREATE FUNCTION advance_activity_domain_evidence_version() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.occurred_at, NEW.occurred_precision, NEW.archived_at, NEW.restricted_at)
 IS DISTINCT FROM ROW(OLD.occurred_at, OLD.occurred_precision, OLD.archived_at, OLD.restricted_at) THEN
  NEW.domain_evidence_version := OLD.domain_evidence_version + 1;
 END IF;
 RETURN NEW;
END;
$$;

CREATE FUNCTION stamp_participant_domain_evidence() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' OR NEW.activity_id IS DISTINCT FROM OLD.activity_id
 OR ROW(NEW.domain_evidence_parent_version, NEW.domain_evidence_at, NEW.domain_evidence_precision, NEW.domain_evidence_visible)
 IS DISTINCT FROM ROW(OLD.domain_evidence_parent_version, OLD.domain_evidence_at, OLD.domain_evidence_precision, OLD.domain_evidence_visible)
 OR NEW.domain_evidence_parent_version IS NULL OR NEW.domain_evidence_at IS NULL
 OR NEW.domain_evidence_precision IS NULL OR NEW.domain_evidence_visible IS NULL THEN
  SELECT a.domain_evidence_version, a.occurred_at, a.occurred_precision,
         a.archived_at IS NULL AND a.restricted_at IS NULL
   INTO NEW.domain_evidence_parent_version, NEW.domain_evidence_at,
        NEW.domain_evidence_precision, NEW.domain_evidence_visible
   FROM activity a WHERE a.id=NEW.activity_id FOR KEY SHARE;
 END IF;
 RETURN NEW;
END;
$$;

CREATE TRIGGER advance_activity_domain_evidence_version
 BEFORE UPDATE OF occurred_at, occurred_precision, archived_at, restricted_at ON activity
 FOR EACH ROW EXECUTE FUNCTION advance_activity_domain_evidence_version();
