-- Tables: consent_event, consent_qualifying_event, contact_acquisition_evidence, provider_run.
SET LOCAL lock_timeout = '5s';

ALTER TABLE consent_event
    DROP CONSTRAINT consent_event_contact_id_fkey,
    ADD CONSTRAINT consent_event_contact_id_fkey FOREIGN KEY (contact_id) REFERENCES contact(id) ON DELETE RESTRICT NOT VALID;

ALTER TABLE consent_qualifying_event
    DROP CONSTRAINT consent_qualifying_event_contact_fkey,
    ADD CONSTRAINT consent_qualifying_event_contact_fkey FOREIGN KEY (contact_id) REFERENCES contact(id) ON DELETE RESTRICT NOT VALID;

ALTER TABLE consent_qualifying_event
    DROP CONSTRAINT consent_qualifying_event_lead_id_fkey,
    ADD CONSTRAINT consent_qualifying_event_lead_id_fkey FOREIGN KEY (lead_id) REFERENCES lead(id) ON DELETE RESTRICT NOT VALID;

ALTER TABLE contact_acquisition_evidence
    DROP CONSTRAINT contact_acquisition_evidence_contact_id_fkey,
    ADD CONSTRAINT contact_acquisition_evidence_contact_id_fkey FOREIGN KEY (contact_id) REFERENCES contact(id) ON DELETE RESTRICT NOT VALID;

ALTER TABLE provider_run
    DROP CONSTRAINT provider_run_contact_id_fkey,
    ADD CONSTRAINT provider_run_contact_id_fkey FOREIGN KEY (contact_id) REFERENCES contact(id) ON DELETE RESTRICT NOT VALID;
