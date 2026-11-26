-- company_fact, company_profile_field and contact_profile_field become read
-- views over field_provenance. Their rows move there through the data task
-- evidence_rows_move_to_field_provenance; until a row has moved,
-- the view reads it from the table's _legacy copy, so no fact disappears from a
-- read in between. A writer moves its subject's legacy rows before it writes.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company_fact RENAME TO company_fact_legacy;
ALTER TABLE company_profile_field RENAME TO company_profile_field_legacy;
ALTER TABLE contact_profile_field RENAME TO contact_profile_field_legacy;

-- A legacy row only moves out, and the move must not count as a change to the
-- company's facts, so the legacy tables keep no row triggers of their own.
DROP TRIGGER trg_company_fact_count_revision ON company_fact_legacy;
DROP TRIGGER trg_company_fact_updated ON company_fact_legacy;
DROP TRIGGER trg_company_profile_field_citation ON company_profile_field_legacy;
DROP TRIGGER trg_company_profile_field_updated ON company_profile_field_legacy;
DROP TRIGGER trg_contact_profile_field_updated ON contact_profile_field_legacy;

-- No writer adds a legacy row, so the vocabularies stand on field_provenance
-- alone.
ALTER TABLE company_fact_legacy
  DROP CONSTRAINT IF EXISTS company_fact_category_check,
  DROP CONSTRAINT IF EXISTS company_fact_field_vocab,
  DROP CONSTRAINT IF EXISTS company_fact_source_check,
  DROP CONSTRAINT IF EXISTS company_fact_value_key_cardinality;
ALTER TABLE company_profile_field_legacy
  DROP CONSTRAINT IF EXISTS company_profile_field_field_check,
  DROP CONSTRAINT IF EXISTS company_profile_field_source_check;
ALTER TABLE contact_profile_field_legacy
  DROP CONSTRAINT IF EXISTS contact_profile_field_field_check,
  DROP CONSTRAINT IF EXISTS contact_profile_field_value_key_cardinality;

CREATE VIEW company_fact AS
  SELECT id, object_id AS company_id, company_fact_category(field_name) AS category, field_name AS field,
         value, value_key, evidence_snippet, evidence_ref AS source_url, confidence, source, captured_by,
         site_read_id, captured_at, version, updated_at, retrieved_at, verified_at, verified_by
    FROM field_provenance
   WHERE object_type = 'company' AND value IS NOT NULL AND company_fact_category(field_name) IS NOT NULL
  UNION ALL
  SELECT id, company_id, category, field, value, value_key, evidence_snippet, source_url, confidence, source,
         captured_by, site_read_id, captured_at, version, updated_at, retrieved_at, verified_at, verified_by
    FROM company_fact_legacy;

CREATE VIEW company_profile_field AS
  SELECT id, object_id AS company_id, field_name AS field, value, evidence_snippet,
         evidence_ref AS source_url, confidence, source, captured_by, captured_at, version, updated_at,
         retrieved_at, verified_at, verified_by, source_activity_id
    FROM field_provenance
   WHERE object_type = 'company' AND value IS NOT NULL AND company_fact_category(field_name) IS NULL
  UNION ALL
  SELECT id, company_id, field, value, evidence_snippet, source_url, confidence, source, captured_by,
         captured_at, version, updated_at, retrieved_at, verified_at, verified_by, source_activity_id
    FROM company_profile_field_legacy;

CREATE VIEW contact_profile_field AS
  SELECT id, object_id AS contact_id, field_name AS field, value, evidence_snippet, evidence_ref AS source_ref,
         confidence, source, captured_by, version, captured_at AS created_at, updated_at,
         coalesce(observed_at, captured_at) AS observed_at, superseded_value, superseded_captured_by, superseded_observed_at, value_key, signature_support
    FROM field_provenance
   WHERE object_type = 'contact' AND value IS NOT NULL
  UNION ALL
  SELECT id, contact_id, field, value, evidence_snippet, source_ref, confidence, source, captured_by, version,
         created_at, updated_at, observed_at, superseded_value, superseded_captured_by, superseded_observed_at,
         value_key, signature_support
    FROM contact_profile_field_legacy;

-- One counter per company, moved by every write of its facts, as the legacy
-- table's trigger did. A row moving out of a legacy table is not a change.
CREATE FUNCTION company_evidence_count_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  touched uuid[] := ARRAY[]::uuid[];
BEGIN
  IF current_setting('margince.evidence_move', true) = 'on' THEN
    RETURN NULL;
  END IF;
  IF TG_OP <> 'DELETE' AND company_fact_category(NEW.field_name) IS NOT NULL THEN
    touched := touched || NEW.object_id;
  END IF;
  IF TG_OP <> 'INSERT' AND company_fact_category(OLD.field_name) IS NOT NULL THEN
    touched := touched || OLD.object_id;
  END IF;
  -- A company being deleted takes its facts with it; it has no counter left
  -- to move.
  INSERT INTO company_fact_revision AS r (company_id, revision)
  SELECT c.id, 1 FROM company c WHERE c.id = ANY(touched)
  ON CONFLICT (company_id) DO UPDATE SET revision = r.revision + 1;
  RETURN NULL;
END;
$$;

-- A cited meeting stays cited only while the meeting is the value's source,
-- whichever writer changes the source.
CREATE FUNCTION evidence_cites_only_a_meeting() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.source IS DISTINCT FROM 'meeting' THEN
    NEW.source_activity_id := NULL;
  END IF;
  RETURN NEW;
END
$$;

-- The typed tables cascaded from their company or contact; a polymorphic row
-- has no foreign key, so the subject's delete removes its evidence here.
CREATE FUNCTION deleted_subject_drops_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM field_provenance f
   USING gone_subject g
   WHERE f.object_type = TG_ARGV[0] AND f.object_id = g.id AND f.value IS NOT NULL;
  RETURN NULL;
END
$$;

CREATE TRIGGER trg_field_provenance_updated
  BEFORE UPDATE ON field_provenance
  FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();
CREATE TRIGGER trg_field_provenance_citation
  BEFORE INSERT OR UPDATE ON field_provenance
  FOR EACH ROW WHEN (NEW.value IS NOT NULL) EXECUTE FUNCTION evidence_cites_only_a_meeting();
CREATE TRIGGER trg_inserted_company_evidence_counts_revision
  AFTER INSERT ON field_provenance
  FOR EACH ROW WHEN (NEW.object_type = 'company' AND NEW.value IS NOT NULL)
  EXECUTE FUNCTION company_evidence_count_revision();
CREATE TRIGGER trg_updated_company_evidence_counts_revision
  AFTER UPDATE ON field_provenance
  FOR EACH ROW WHEN ((NEW.object_type = 'company' AND NEW.value IS NOT NULL)
                  OR (OLD.object_type = 'company' AND OLD.value IS NOT NULL))
  EXECUTE FUNCTION company_evidence_count_revision();
CREATE TRIGGER trg_deleted_company_evidence_counts_revision
  AFTER DELETE ON field_provenance
  FOR EACH ROW WHEN (OLD.object_type = 'company' AND OLD.value IS NOT NULL)
  EXECUTE FUNCTION company_evidence_count_revision();
CREATE TRIGGER deleted_company_drops_evidence
  AFTER DELETE ON company REFERENCING OLD TABLE AS gone_subject
  FOR EACH STATEMENT EXECUTE FUNCTION deleted_subject_drops_evidence('company');
CREATE TRIGGER deleted_contact_drops_evidence
  AFTER DELETE ON contact REFERENCING OLD TABLE AS gone_subject
  FOR EACH STATEMENT EXECUTE FUNCTION deleted_subject_drops_evidence('contact');
