-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- An approval's two target type columns held any text. They now hold the list
-- approvals.TargetType declares, or a unit's own table (ext_<unit>_<name>).
-- A contact's acquisition evidence names the kind of record that evidences it, an
-- activity or a deal, as consent_qualifying_event already restricts its twin column.
-- Added NOT VALID and validated in the file beside this one.
SET LOCAL lock_timeout = '5s';

ALTER TABLE approval
    ADD CONSTRAINT approval_target_entity_type_check CHECK (
        target_entity_type = ANY (ARRAY['contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template', 'offer', 'commission', 'deal_room', 'deal_room_thread', 'deal_room_comment', 'deal_room_document', 'deal_room_participant', 'list', 'signal', 'saved_view', 'tag', 'custom_field', 'webhook_subscription', 'import_run', 'report_definition', 'app_user', 'user_admin', 'team', 'record_grant', 'data_subject_request', 'fx_rate', 'ai_model_rate', 'legal_review', 'brief', 'stage'])
        OR target_entity_type ~ '^ext_[a-z0-9]+(_[a-z0-9]+)*$') NOT VALID,
    ADD CONSTRAINT approval_co_target_entity_type_check CHECK (
        co_target_entity_type = ANY (ARRAY['contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template', 'offer', 'commission', 'deal_room', 'deal_room_thread', 'deal_room_comment', 'deal_room_document', 'deal_room_participant', 'list', 'signal', 'saved_view', 'tag', 'custom_field', 'webhook_subscription', 'import_run', 'report_definition', 'app_user', 'user_admin', 'team', 'record_grant', 'data_subject_request', 'fx_rate', 'ai_model_rate', 'legal_review', 'brief', 'stage'])
        OR co_target_entity_type ~ '^ext_[a-z0-9]+(_[a-z0-9]+)*$') NOT VALID;

ALTER TABLE contact_acquisition_evidence
    ADD CONSTRAINT contact_acquisition_evidence_source_entity_type_check CHECK (
        source_entity_type IN ('activity', 'deal')) NOT VALID;
