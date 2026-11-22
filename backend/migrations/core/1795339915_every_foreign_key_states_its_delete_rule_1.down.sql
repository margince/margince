-- Tables: activity, ai_call, consent_event, contact_channel_identity, deal.
-- Restores the foreign keys on these tables without a stated delete rule.
-- Rollback restores the rule for new rows only: the keys come back NOT VALID,
-- because a down has no later file to validate them in.
SET LOCAL lock_timeout = '5s';

ALTER TABLE activity
    DROP CONSTRAINT activity_channel_provider_fkey,
    ADD CONSTRAINT activity_channel_provider_fkey FOREIGN KEY (channel_provider) REFERENCES channel_provider(provider) NOT VALID,
    DROP CONSTRAINT activity_kind_fkey,
    ADD CONSTRAINT activity_kind_fkey FOREIGN KEY (kind) REFERENCES activity_kind(kind) NOT VALID;
ALTER TABLE ai_call
    DROP CONSTRAINT ai_call_config_fk,
    ADD CONSTRAINT ai_call_config_fk FOREIGN KEY (config_hash) REFERENCES ai_call_config(hash) NOT VALID;
ALTER TABLE consent_event
    DROP CONSTRAINT consent_event_consent_text_version_id_fkey,
    ADD CONSTRAINT consent_event_consent_text_version_id_fkey FOREIGN KEY (consent_text_version_id) REFERENCES consent_text_version(id) NOT VALID,
    DROP CONSTRAINT consent_event_lead_id_fkey,
    ADD CONSTRAINT consent_event_lead_id_fkey FOREIGN KEY (lead_id) REFERENCES lead(id) NOT VALID;
ALTER TABLE contact_channel_identity
    DROP CONSTRAINT contact_channel_identity_provider_fkey,
    ADD CONSTRAINT contact_channel_identity_provider_fkey FOREIGN KEY (provider) REFERENCES channel_provider(provider) NOT VALID;
ALTER TABLE deal
    DROP CONSTRAINT deal_stage_in_pipeline,
    ADD CONSTRAINT deal_stage_in_pipeline FOREIGN KEY (stage_id, pipeline_id) REFERENCES stage(id, pipeline_id) NOT VALID;
