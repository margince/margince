SET LOCAL lock_timeout = '5s';

-- Every column is new, so each row holds the default or NULL and passes.
ALTER TABLE consent_purpose VALIDATE CONSTRAINT consent_purpose_frequency_check;
ALTER TABLE contact_acquisition_evidence VALIDATE CONSTRAINT contact_acquisition_evidence_stated_source_check;
ALTER TABLE confirm_token VALIDATE CONSTRAINT confirm_token_subject_request_id_fkey;
ALTER TABLE consent_event VALIDATE CONSTRAINT consent_event_subject_request_id_fkey;
