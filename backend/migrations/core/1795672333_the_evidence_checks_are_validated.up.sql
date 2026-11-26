-- The checks and keys added to field_provenance beside the evidence columns are
-- validated here, in their own transaction, so the scan runs under SHARE UPDATE
-- EXCLUSIVE.
SET LOCAL lock_timeout = '5s';
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_site_read_fkey;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_source_activity_fkey;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_evidence_subject;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_stamp_has_no_key;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_company_profile_field;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_contact_profile_field;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_evidence_value_key;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_company_profile_source;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_company_fact_source;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_site_evidence;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_technical_evidence;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_meeting_evidence;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_contact_evidence_quote;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_verified_pair;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_version_js_safe;
