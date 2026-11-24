SET LOCAL lock_timeout = '3s';

DELETE FROM confirm_token WHERE lead_id IS NOT NULL AND kind = 'consent_confirmation';
ALTER TABLE confirm_token DROP CONSTRAINT confirm_token_lead_is_only_told;
ALTER TABLE confirm_token
    ADD CONSTRAINT confirm_token_lead_is_only_told
        CHECK (lead_id IS NULL OR kind = 'privacy_notice') NOT VALID;
