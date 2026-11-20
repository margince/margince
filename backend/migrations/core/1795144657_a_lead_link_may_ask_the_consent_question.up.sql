-- A lead can hold a consent link as well as a privacy notice, so the double
-- opt-in a booking asks for works for a lead exactly as it does for a contact.
SET LOCAL lock_timeout = '3s';

ALTER TABLE confirm_token DROP CONSTRAINT confirm_token_lead_is_only_told;
ALTER TABLE confirm_token
    ADD CONSTRAINT confirm_token_lead_is_only_told
        CHECK (lead_id IS NULL OR kind IN ('privacy_notice', 'consent_confirmation')) NOT VALID;
