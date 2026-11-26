-- field_provenance becomes the one table that records what Margince believes
-- about a company or contact field: the value, its evidence, its confidence and
-- the value it replaced. A row with a value is evidence; a row without one stays
-- the stamp of who wrote a record column.
--
-- The checks are NOT VALID here and validated in a later file, so the scan runs
-- under SHARE UPDATE EXCLUSIVE rather than blocking writes.
SET LOCAL lock_timeout = '3s';

ALTER TABLE field_provenance
  ADD COLUMN value text,
  ADD COLUMN value_key text NOT NULL DEFAULT '',
  ADD COLUMN evidence_snippet text,
  ADD COLUMN retrieved_at timestamptz,
  ADD COLUMN observed_at timestamptz,
  ADD COLUMN verified_at timestamptz,
  ADD COLUMN verified_by uuid,
  ADD COLUMN superseded_value text,
  ADD COLUMN superseded_captured_by text,
  ADD COLUMN superseded_observed_at timestamptz,
  ADD COLUMN signature_support jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN site_read_id uuid,
  ADD COLUMN source_activity_id uuid,
  ADD COLUMN version bigint NOT NULL DEFAULT 1,
  ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE field_provenance
  ADD CONSTRAINT field_provenance_site_read_fkey FOREIGN KEY (site_read_id)
    REFERENCES site_read(id) ON DELETE SET NULL (site_read_id) NOT VALID,
  ADD CONSTRAINT field_provenance_source_activity_fkey FOREIGN KEY (source_activity_id)
    REFERENCES activity(id) ON DELETE CASCADE NOT VALID;

-- The category a company fact files under follows from its field alone, so it
-- is derived here rather than stored. NULL names a company profile field.
CREATE FUNCTION company_fact_category(field text) RETURNS text
  LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT CASE
    WHEN field IN ('founded_year', 'employee_range', 'phone', 'contact_email', 'location') THEN 'company'
    WHEN field IN ('service', 'product', 'capability') THEN 'offering'
    WHEN field IN ('served_industry', 'company_size', 'geography', 'language') THEN 'market'
    WHEN field IN ('certification', 'partner', 'named_customer', 'technology', 'quantified_outcome',
                   'mail_provider', 'email_security', 'hosting_provider', 'operated_service') THEN 'signal'
  END
$$;

ALTER TABLE field_provenance
  ADD CONSTRAINT field_provenance_evidence_subject
    CHECK (value IS NULL OR object_type IN ('company', 'contact')) NOT VALID,
  ADD CONSTRAINT field_provenance_stamp_has_no_key
    CHECK (value IS NOT NULL OR value_key = '') NOT VALID,
  -- Each list below is the whole vocabulary of one evidence view's column. A
  -- company field is a fact field, which company_fact_category names, or one
  -- of the profile fields listed here.
  ADD CONSTRAINT field_provenance_company_profile_field
    CHECK (value IS NULL OR object_type <> 'company' OR company_fact_category(field_name) IS NOT NULL
      OR field_name = ANY (ARRAY[
      'display_name', 'offer_summary', 'icp', 'value_proposition', 'usp', 'customer_pains',
      'desired_outcomes', 'buying_center', 'buying_intents', 'common_objections', 'sales_motion',
      'legal_name', 'registered_address', 'register_vat', 'industry', 'history', 'legal_form',
      'register_court', 'register_number'])) NOT VALID,
  ADD CONSTRAINT field_provenance_contact_profile_field
    CHECK (value IS NULL OR object_type <> 'contact' OR field_name = ANY (ARRAY[
      'title', 'phone', 'role', 'linkedin', 'company_name', 'address', 'website',
      'first_name', 'last_name', 'email'])) NOT VALID,
  -- A field that holds several values keys each one; a single-valued field
  -- holds the empty key, so its one row is the field's answer.
  ADD CONSTRAINT field_provenance_evidence_value_key
    CHECK (value IS NULL
      OR (object_type = 'company' AND (company_fact_category(field_name) IS NULL
            OR (company_fact_category(field_name) = 'company' AND field_name <> 'location'))
          AND value_key = '')
      OR (object_type = 'company' AND (field_name = 'location'
            OR company_fact_category(field_name) IN ('offering', 'market', 'signal'))
          AND value_key <> '')
      OR (object_type = 'contact' AND (field_name IN ('phone', 'email')) = (value_key <> ''))) NOT VALID,
  ADD CONSTRAINT field_provenance_company_profile_source
    CHECK (value IS NULL OR object_type <> 'company' OR company_fact_category(field_name) IS NOT NULL
      OR source = ANY (ARRAY['human', 'agent', 'site_read', 'connector', 'migration', 'meeting'])) NOT VALID,
  ADD CONSTRAINT field_provenance_company_fact_source
    CHECK (value IS NULL OR object_type <> 'company' OR company_fact_category(field_name) IS NULL
      OR source = ANY (ARRAY['human', 'agent', 'site_read', 'connector', 'migration', 'technical_lookup'])) NOT VALID,
  ADD CONSTRAINT field_provenance_site_evidence
    CHECK (value IS NULL OR object_type <> 'company' OR source <> 'site_read'
      OR (coalesce(evidence_snippet, '') <> '' AND coalesce(evidence_ref, '') <> ''
          AND confidence IS NOT NULL)) NOT VALID,
  ADD CONSTRAINT field_provenance_technical_evidence
    CHECK (value IS NULL OR source <> 'technical_lookup'
      OR (coalesce(evidence_snippet, '') <> '' AND coalesce(evidence_ref, '') <> ''
          AND retrieved_at IS NOT NULL)) NOT VALID,
  ADD CONSTRAINT field_provenance_meeting_evidence
    CHECK (value IS NULL OR source <> 'meeting'
      OR (source_activity_id IS NOT NULL AND confidence IS NOT NULL
          AND coalesce(evidence_snippet, '') = '')) NOT VALID,
  -- A contact field is always read off something: a card, a signature, a page.
  ADD CONSTRAINT field_provenance_contact_evidence_quote
    CHECK (value IS NULL OR object_type <> 'contact'
      OR (evidence_snippet IS NOT NULL AND evidence_ref IS NOT NULL)) NOT VALID,
  ADD CONSTRAINT field_provenance_verified_pair
    CHECK ((verified_at IS NULL) = (verified_by IS NULL)) NOT VALID,
  ADD CONSTRAINT field_provenance_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
