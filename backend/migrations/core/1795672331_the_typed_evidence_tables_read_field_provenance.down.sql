SET LOCAL lock_timeout = '3s';

DROP TRIGGER deleted_contact_drops_evidence ON contact;
DROP TRIGGER deleted_company_drops_evidence ON company;
DROP TRIGGER trg_deleted_company_evidence_counts_revision ON field_provenance;
DROP TRIGGER trg_updated_company_evidence_counts_revision ON field_provenance;
DROP TRIGGER trg_inserted_company_evidence_counts_revision ON field_provenance;
DROP TRIGGER trg_field_provenance_citation ON field_provenance;
DROP TRIGGER trg_field_provenance_updated ON field_provenance;
DROP FUNCTION deleted_subject_drops_evidence();
DROP FUNCTION evidence_cites_only_a_meeting();
DROP FUNCTION company_evidence_count_revision();

-- The moved rows go back to their typed tables, under the ids they had.
INSERT INTO company_fact_legacy (id, company_id, category, field, value, value_key, evidence_snippet, source_url,
       confidence, source, captured_by, site_read_id, captured_at, version, updated_at, retrieved_at, verified_at,
       verified_by)
SELECT id, company_id, category, field, value, value_key, evidence_snippet, source_url, confidence, source,
       captured_by, site_read_id, captured_at, version, updated_at, retrieved_at, verified_at, verified_by
  FROM company_fact f WHERE NOT EXISTS (SELECT 1 FROM company_fact_legacy l WHERE l.id = f.id);
INSERT INTO company_profile_field_legacy (id, company_id, field, value, evidence_snippet, source_url, confidence,
       source, captured_by, captured_at, version, updated_at, retrieved_at, verified_at, verified_by,
       source_activity_id)
SELECT id, company_id, field, value, evidence_snippet, source_url, confidence, source, captured_by, captured_at,
       version, updated_at, retrieved_at, verified_at, verified_by, source_activity_id
  FROM company_profile_field f WHERE NOT EXISTS (SELECT 1 FROM company_profile_field_legacy l WHERE l.id = f.id);
INSERT INTO contact_profile_field_legacy (id, contact_id, field, value, evidence_snippet, source_ref, confidence,
       source, captured_by, version, created_at, updated_at, observed_at, superseded_value, superseded_captured_by,
       superseded_observed_at, value_key, signature_support)
SELECT id, contact_id, field, value, evidence_snippet, source_ref, confidence, source, captured_by, version,
       created_at, updated_at, observed_at, superseded_value, superseded_captured_by, superseded_observed_at,
       value_key, signature_support
  FROM contact_profile_field f WHERE NOT EXISTS (SELECT 1 FROM contact_profile_field_legacy l WHERE l.id = f.id);
DELETE FROM field_provenance WHERE value IS NOT NULL;

DROP VIEW contact_profile_field;
DROP VIEW company_profile_field;
DROP VIEW company_fact;

ALTER TABLE contact_profile_field_legacy RENAME TO contact_profile_field;
ALTER TABLE company_profile_field_legacy RENAME TO company_profile_field;
ALTER TABLE company_fact_legacy RENAME TO company_fact;

ALTER TABLE company_fact ADD CONSTRAINT company_fact_category_check CHECK ((category = ANY (ARRAY['company'::text, 'offering'::text, 'market'::text, 'signal'::text]))) NOT VALID;
ALTER TABLE company_fact ADD CONSTRAINT company_fact_field_vocab CHECK ((((category = 'company'::text) AND (field = ANY (ARRAY['founded_year'::text, 'employee_range'::text, 'phone'::text, 'contact_email'::text, 'location'::text]))) OR ((category = 'offering'::text) AND (field = ANY (ARRAY['service'::text, 'product'::text, 'capability'::text]))) OR ((category = 'market'::text) AND (field = ANY (ARRAY['served_industry'::text, 'company_size'::text, 'geography'::text, 'language'::text]))) OR ((category = 'signal'::text) AND (field = ANY (ARRAY['certification'::text, 'partner'::text, 'named_customer'::text, 'technology'::text, 'quantified_outcome'::text, 'mail_provider'::text, 'email_security'::text, 'hosting_provider'::text, 'operated_service'::text]))))) NOT VALID;
ALTER TABLE company_fact ADD CONSTRAINT company_fact_source_check CHECK ((source = ANY (ARRAY['human'::text, 'agent'::text, 'site_read'::text, 'connector'::text, 'migration'::text, 'technical_lookup'::text]))) NOT VALID;
ALTER TABLE company_fact ADD CONSTRAINT company_fact_value_key_cardinality CHECK ((((category = 'company'::text) AND (field <> 'location'::text) AND (value_key = ''::text)) OR ((category = 'company'::text) AND (field = 'location'::text) AND (value_key <> ''::text)) OR ((category = ANY (ARRAY['offering'::text, 'market'::text, 'signal'::text])) AND (value_key <> ''::text)))) NOT VALID;
ALTER TABLE company_profile_field ADD CONSTRAINT company_profile_field_field_check CHECK ((field = ANY (ARRAY['display_name'::text, 'offer_summary'::text, 'icp'::text, 'value_proposition'::text, 'usp'::text, 'customer_pains'::text, 'desired_outcomes'::text, 'buying_center'::text, 'buying_intents'::text, 'common_objections'::text, 'sales_motion'::text, 'legal_name'::text, 'registered_address'::text, 'register_vat'::text, 'industry'::text, 'history'::text, 'legal_form'::text, 'register_court'::text, 'register_number'::text]))) NOT VALID;
ALTER TABLE company_profile_field ADD CONSTRAINT company_profile_field_source_check CHECK ((source = ANY (ARRAY['human'::text, 'agent'::text, 'site_read'::text, 'connector'::text, 'migration'::text, 'meeting'::text]))) NOT VALID;
ALTER TABLE contact_profile_field ADD CONSTRAINT contact_profile_field_field_check CHECK ((field = ANY (ARRAY['title'::text, 'phone'::text, 'role'::text, 'linkedin'::text, 'company_name'::text, 'address'::text, 'website'::text, 'first_name'::text, 'last_name'::text, 'email'::text]))) NOT VALID;
ALTER TABLE contact_profile_field ADD CONSTRAINT contact_profile_field_value_key_cardinality CHECK (((field = ANY (ARRAY['phone'::text, 'email'::text])) = (value_key <> ''::text))) NOT VALID;

CREATE TRIGGER trg_company_fact_count_revision
  AFTER INSERT OR UPDATE OR DELETE ON company_fact
  FOR EACH ROW EXECUTE FUNCTION company_fact_count_revision();
CREATE TRIGGER trg_company_fact_updated
  BEFORE UPDATE ON company_fact FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();
CREATE TRIGGER trg_company_profile_field_citation
  BEFORE INSERT OR UPDATE ON company_profile_field
  FOR EACH ROW EXECUTE FUNCTION company_profile_field_cites_only_a_meeting();
CREATE TRIGGER trg_company_profile_field_updated
  BEFORE UPDATE ON company_profile_field FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();
CREATE TRIGGER trg_contact_profile_field_updated
  BEFORE UPDATE ON contact_profile_field FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();
