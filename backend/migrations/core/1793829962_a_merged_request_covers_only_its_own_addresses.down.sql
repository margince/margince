SET LOCAL lock_timeout = '3s';

ALTER TABLE consent_qualifying_event DROP COLUMN recorded_for_addresses;
ALTER TABLE contact_acquisition_evidence DROP COLUMN recorded_for_addresses;
