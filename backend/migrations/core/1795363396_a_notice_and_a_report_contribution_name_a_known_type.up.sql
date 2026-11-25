-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- notice.target_type held any text. It now holds what an approval can name (the
-- list approvals.TargetType declares, or a unit's own table), or one of the
-- things only a notice points at (notices.OwnTargetType).
-- report_edition_contribution.source_type held any text. It now holds the list
-- reporting.SourceType declares, one source for each way a metric counts.
-- Added NOT VALID and validated in the file beside this one.
SET LOCAL lock_timeout = '5s';

ALTER TABLE notice
    ADD CONSTRAINT notice_target_type_check CHECK (
        target_type = ANY (ARRAY['contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template', 'offer', 'commission', 'deal_room', 'deal_room_thread', 'deal_room_comment', 'deal_room_document', 'deal_room_participant', 'list', 'signal', 'saved_view', 'tag', 'custom_field', 'webhook_subscription', 'import_run', 'report_definition', 'app_user', 'user_admin', 'team', 'record_grant', 'data_subject_request', 'fx_rate', 'ai_model_rate', 'legal_review', 'brief', 'stage', 'approval', 'data_task', 'retained_email_signature', 'email_signature', 'signature_template', 'legal_entity', 'knowledge_document', 'approval_history'])
        OR target_type ~ '^ext_[a-z0-9]+(_[a-z0-9]+)*$') NOT VALID;

ALTER TABLE report_edition_contribution
    ADD CONSTRAINT report_edition_contribution_source_type_check CHECK (
        source_type = ANY (ARRAY['deal', 'activity', 'lead', 'company', 'sdr_handoff', 'intro_request', 'forecast_projection'])) NOT VALID;
