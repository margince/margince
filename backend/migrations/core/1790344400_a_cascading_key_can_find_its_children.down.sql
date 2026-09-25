-- Dropping an index takes an ACCESS EXCLUSIVE lock on its table, so the wait is
-- bounded here for the same reason the up half bounds its own.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_activity_request_settlement_judged_through_activity;
DROP INDEX IF EXISTS idx_analytics_share_snapshot;
DROP INDEX IF EXISTS idx_capture_exclusion_user;
DROP INDEX IF EXISTS idx_capture_pending_counterparty_activity;
DROP INDEX IF EXISTS idx_capture_pending_counterparty_owner;
DROP INDEX IF EXISTS idx_close_date_run_member_deal;
DROP INDEX IF EXISTS idx_communication_decision_delivery;
DROP INDEX IF EXISTS idx_communication_suppression_lead;
DROP INDEX IF EXISTS idx_company_domain_disposition_company;
DROP INDEX IF EXISTS idx_contact_confirm_submission_token;
DROP INDEX IF EXISTS idx_contact_signature_enrich_state_activity;
DROP INDEX IF EXISTS idx_deal_document_hide_attachment;
DROP INDEX IF EXISTS idx_deal_room_comment_room;
DROP INDEX IF EXISTS idx_deal_room_engagement_participant_room;
DROP INDEX IF EXISTS idx_deal_room_session_room;
DROP INDEX IF EXISTS idx_deal_room_session_participant_room;
DROP INDEX IF EXISTS idx_dedupe_candidate_left_company;
DROP INDEX IF EXISTS idx_dedupe_candidate_left_lead;
DROP INDEX IF EXISTS idx_dedupe_candidate_right_lead;
DROP INDEX IF EXISTS idx_dedupe_candidate_left_contact;
DROP INDEX IF EXISTS idx_dedupe_candidate_right_contact;
DROP INDEX IF EXISTS idx_dedupe_candidate_right_company;
DROP INDEX IF EXISTS idx_linkedin_connection_matched_contact;
DROP INDEX IF EXISTS idx_oauth_authorization_code_user;
DROP INDEX IF EXISTS idx_privacy_notice_case_contact;
DROP INDEX IF EXISTS idx_provider_applied_field_run_contact_provider;
DROP INDEX IF EXISTS idx_role_assignment_team;
DROP INDEX IF EXISTS idx_stage_progression_outcome_from_stage;
DROP INDEX IF EXISTS idx_stage_progression_outcome_to_stage;
DROP INDEX IF EXISTS idx_stage_progression_policy_to_stage;
DROP INDEX IF EXISTS idx_stage_progression_policy_from_stage;
DROP INDEX IF EXISTS idx_webhook_subscription_owner;
DROP INDEX IF EXISTS idx_withdrawal_credential_purpose;
