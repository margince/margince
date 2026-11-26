-- The constraints added NOT VALID by the file beside this one are validated here, in
-- their own transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '5s';

ALTER TABLE activity VALIDATE CONSTRAINT activity_channel_provider_fkey;
ALTER TABLE activity VALIDATE CONSTRAINT activity_kind_fkey;
ALTER TABLE ai_call VALIDATE CONSTRAINT ai_call_config_fk;
ALTER TABLE consent_event VALIDATE CONSTRAINT consent_event_consent_text_version_id_fkey;
ALTER TABLE consent_event VALIDATE CONSTRAINT consent_event_lead_id_fkey;
ALTER TABLE contact_channel_identity VALIDATE CONSTRAINT contact_channel_identity_provider_fkey;
ALTER TABLE deal VALIDATE CONSTRAINT deal_stage_in_pipeline;
