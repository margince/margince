SET LOCAL lock_timeout = '5s';

-- Both columns are new, so every row holds the default or NULL and passes.
ALTER TABLE consent_purpose VALIDATE CONSTRAINT consent_purpose_frequency_check;
ALTER TABLE contact_acquisition_evidence VALIDATE CONSTRAINT contact_acquisition_evidence_stated_source_check;
