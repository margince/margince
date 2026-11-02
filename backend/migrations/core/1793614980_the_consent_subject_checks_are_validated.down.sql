-- Put both checks back the way this migration found them: present, unvalidated.
-- Postgres has no ALTER that un-validates, so each is dropped and re-added
-- NOT VALID in one statement, leaving no window with the rule absent.
SET LOCAL lock_timeout = '3s';

ALTER TABLE consent_event
    DROP CONSTRAINT consent_event_subject,
    ADD CONSTRAINT consent_event_subject
        CHECK ((contact_id IS NULL) <> (lead_id IS NULL)) NOT VALID;

ALTER TABLE contact_consent
    DROP CONSTRAINT contact_consent_subject,
    ADD CONSTRAINT contact_consent_subject
        CHECK ((contact_id IS NULL) <> (lead_id IS NULL)) NOT VALID;
