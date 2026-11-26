SET LOCAL lock_timeout = '3s';

ALTER TABLE field_provenance
  DROP CONSTRAINT field_provenance_version_js_safe,
  DROP CONSTRAINT field_provenance_verified_pair,
  DROP CONSTRAINT field_provenance_contact_evidence_quote,
  DROP CONSTRAINT field_provenance_meeting_evidence,
  DROP CONSTRAINT field_provenance_technical_evidence,
  DROP CONSTRAINT field_provenance_site_evidence,
  DROP CONSTRAINT field_provenance_company_fact_source,
  DROP CONSTRAINT field_provenance_company_profile_source,
  DROP CONSTRAINT field_provenance_evidence_value_key,
  DROP CONSTRAINT field_provenance_contact_profile_field,
  DROP CONSTRAINT field_provenance_company_profile_field,
  DROP CONSTRAINT field_provenance_stamp_has_no_key,
  DROP CONSTRAINT field_provenance_evidence_subject;

DROP FUNCTION company_fact_category(text);

ALTER TABLE field_provenance
  DROP COLUMN updated_at,
  DROP COLUMN version,
  DROP COLUMN source_activity_id,
  DROP COLUMN site_read_id,
  DROP COLUMN signature_support,
  DROP COLUMN superseded_observed_at,
  DROP COLUMN superseded_captured_by,
  DROP COLUMN superseded_value,
  DROP COLUMN verified_by,
  DROP COLUMN verified_at,
  DROP COLUMN observed_at,
  DROP COLUMN retrieved_at,
  DROP COLUMN evidence_snippet,
  DROP COLUMN value_key,
  DROP COLUMN value;
