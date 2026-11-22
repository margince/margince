-- Restores the foreign keys without a stated delete rule.
SET LOCAL lock_timeout = '5s';

ALTER TABLE activity
    DROP CONSTRAINT activity_channel_provider_fkey,
    ADD CONSTRAINT activity_channel_provider_fkey FOREIGN KEY (channel_provider) REFERENCES channel_provider(provider),
    DROP CONSTRAINT activity_kind_fkey,
    ADD CONSTRAINT activity_kind_fkey FOREIGN KEY (kind) REFERENCES activity_kind(kind);
ALTER TABLE ai_call
    DROP CONSTRAINT ai_call_config_fk,
    ADD CONSTRAINT ai_call_config_fk FOREIGN KEY (config_hash) REFERENCES ai_call_config(hash);
ALTER TABLE consent_event
    DROP CONSTRAINT consent_event_consent_text_version_id_fkey,
    ADD CONSTRAINT consent_event_consent_text_version_id_fkey FOREIGN KEY (consent_text_version_id) REFERENCES consent_text_version(id),
    DROP CONSTRAINT consent_event_lead_id_fkey,
    ADD CONSTRAINT consent_event_lead_id_fkey FOREIGN KEY (lead_id) REFERENCES lead(id);
ALTER TABLE contact_channel_identity
    DROP CONSTRAINT contact_channel_identity_provider_fkey,
    ADD CONSTRAINT contact_channel_identity_provider_fkey FOREIGN KEY (provider) REFERENCES channel_provider(provider);
ALTER TABLE deal
    DROP CONSTRAINT deal_stage_in_pipeline,
    ADD CONSTRAINT deal_stage_in_pipeline FOREIGN KEY (stage_id, pipeline_id) REFERENCES stage(id, pipeline_id);
ALTER TABLE deal_product
    DROP CONSTRAINT deal_product_product_id_fkey,
    ADD CONSTRAINT deal_product_product_id_fkey FOREIGN KEY (product_id) REFERENCES product(id);
ALTER TABLE field_mask
    DROP CONSTRAINT field_mask_maskable_fkey,
    ADD CONSTRAINT field_mask_maskable_fkey FOREIGN KEY (object, field) REFERENCES maskable_field(object, field);
ALTER TABLE forecast_snapshot
    DROP CONSTRAINT forecast_snapshot_pipeline_id_fkey,
    ADD CONSTRAINT forecast_snapshot_pipeline_id_fkey FOREIGN KEY (pipeline_id) REFERENCES pipeline(id);
ALTER TABLE intro_request
    DROP CONSTRAINT intro_request_through_contact_id_fkey,
    ADD CONSTRAINT intro_request_through_contact_id_fkey FOREIGN KEY (through_contact_id) REFERENCES contact(id);
ALTER TABLE lead
    DROP CONSTRAINT lead_product_id_fkey,
    ADD CONSTRAINT lead_product_id_fkey FOREIGN KEY (product_id) REFERENCES product(id);
ALTER TABLE report_definition
    DROP CONSTRAINT report_definition_audience_team_id_fkey,
    ADD CONSTRAINT report_definition_audience_team_id_fkey FOREIGN KEY (audience_team_id) REFERENCES team(id);
ALTER TABLE report_definition_revision
    DROP CONSTRAINT report_definition_revision_report_id_fkey,
    ADD CONSTRAINT report_definition_revision_report_id_fkey FOREIGN KEY (report_id) REFERENCES report_definition(id);
ALTER TABLE report_edition
    DROP CONSTRAINT report_edition_execution_id_fkey,
    ADD CONSTRAINT report_edition_execution_id_fkey FOREIGN KEY (execution_id) REFERENCES report_execution(id),
    DROP CONSTRAINT report_edition_report_id_fkey,
    ADD CONSTRAINT report_edition_report_id_fkey FOREIGN KEY (report_id) REFERENCES report_definition(id);
ALTER TABLE report_execution
    DROP CONSTRAINT report_execution_report_id_report_revision_fkey,
    ADD CONSTRAINT report_execution_report_id_report_revision_fkey FOREIGN KEY (report_id, report_revision) REFERENCES report_definition_revision(report_id, revision),
    DROP CONSTRAINT report_execution_schedule_id_fkey,
    ADD CONSTRAINT report_execution_schedule_id_fkey FOREIGN KEY (schedule_id) REFERENCES report_schedule(id);
ALTER TABLE report_schedule
    DROP CONSTRAINT report_schedule_report_id_report_revision_fkey,
    ADD CONSTRAINT report_schedule_report_id_report_revision_fkey FOREIGN KEY (report_id, report_revision) REFERENCES report_definition_revision(report_id, revision);
ALTER TABLE sales_target
    DROP CONSTRAINT sales_target_pipeline_id_fkey,
    ADD CONSTRAINT sales_target_pipeline_id_fkey FOREIGN KEY (pipeline_id) REFERENCES pipeline(id);
ALTER TABLE sales_target_revision
    DROP CONSTRAINT sales_target_revision_target_id_fkey,
    ADD CONSTRAINT sales_target_revision_target_id_fkey FOREIGN KEY (target_id) REFERENCES sales_target(id);
ALTER TABLE voice_build
    DROP CONSTRAINT voice_build_result_version_fkey,
    ADD CONSTRAINT voice_build_result_version_fkey FOREIGN KEY (voice_profile_id, result_version) REFERENCES voice_profile_version(voice_profile_id, profile_version);
ALTER TABLE voice_learning_signal
    DROP CONSTRAINT voice_learning_signal_version_fkey,
    ADD CONSTRAINT voice_learning_signal_version_fkey FOREIGN KEY (voice_profile_id, profile_version) REFERENCES voice_profile_version(voice_profile_id, profile_version);
ALTER TABLE voice_profile_delta
    DROP CONSTRAINT voice_profile_delta_from_fkey,
    ADD CONSTRAINT voice_profile_delta_from_fkey FOREIGN KEY (voice_profile_id, from_version) REFERENCES voice_profile_version(voice_profile_id, profile_version),
    DROP CONSTRAINT voice_profile_delta_to_fkey,
    ADD CONSTRAINT voice_profile_delta_to_fkey FOREIGN KEY (voice_profile_id, to_version) REFERENCES voice_profile_version(voice_profile_id, profile_version);
ALTER TABLE voice_profile_version
    DROP CONSTRAINT voice_profile_version_predecessor_fkey,
    ADD CONSTRAINT voice_profile_version_predecessor_fkey FOREIGN KEY (voice_profile_id, predecessor_version) REFERENCES voice_profile_version(voice_profile_id, profile_version);
