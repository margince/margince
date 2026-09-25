-- An index on every ON DELETE CASCADE foreign key's referencing columns.
--
-- Postgres has to FIND the child rows before it can cascade a delete. With no
-- index on the referencing column that is a sequential scan of the child table,
-- performed inside the parent's delete transaction and holding its locks.
--
-- Two places this is not theoretical. `dedupe_candidate` cascades from six
-- directions — contact, company and lead, each on a left and a right column —
-- so deleting any one of those record types scans the whole dedupe queue six
-- times in one transaction. And Article 17 erasure deletes contacts by design,
-- running against dedupe_candidate twice, privacy_notice_case,
-- linkedin_connection and contact_signature_enrich_state at once.
--
-- ONLY the cascading keys. The schema has far more unindexed foreign keys than
-- these, and most of them are correct as they are: the column exists for
-- integrity, nothing asks the reverse question, and an index nobody reads is
-- pure write cost. A cascade is different because the DELETE itself is the
-- reader.
--
-- Composite keys are indexed in the constraint's own column order, so the
-- cascade's lookup matches the index prefix rather than merely overlapping it.
--
-- Cheapest now: these tables are empty, so each CREATE INDEX is instant. On a
-- populated one it is a CREATE INDEX CONCURRENTLY, a maintenance window, and a
-- chance of failing halfway.
--
-- Held by TestEveryCascadingKeyCanFindItsChildren, which derives the same set
-- from pg_constraint rather than from this list.

-- CREATE INDEX takes a SHARE lock, which blocks writers on a table this
-- migration did not create. Bounded: failing fast is recoverable, and stalling
-- every write to dedupe_candidate behind one open transaction is not.
SET LOCAL lock_timeout = '3s';
CREATE INDEX idx_activity_request_settlement_judged_through_activity ON activity_request_settlement (judged_through_activity_id);
CREATE INDEX idx_analytics_share_snapshot ON analytics_share (snapshot_id);
CREATE INDEX idx_capture_exclusion_user ON capture_exclusion (user_id);
CREATE INDEX idx_capture_pending_counterparty_activity ON capture_pending_counterparty (activity_id);
CREATE INDEX idx_capture_pending_counterparty_owner ON capture_pending_counterparty (owner_id);
CREATE INDEX idx_close_date_run_member_deal ON close_date_run_member (deal_id);
CREATE INDEX idx_communication_decision_delivery ON communication_decision (delivery_id);
CREATE INDEX idx_communication_suppression_lead ON communication_suppression (lead_id);
CREATE INDEX idx_company_domain_disposition_company ON company_domain_disposition (company_id);
CREATE INDEX idx_contact_confirm_submission_token ON contact_confirm_submission (token_id);
CREATE INDEX idx_contact_signature_enrich_state_activity ON contact_signature_enrich_state (activity_id);
CREATE INDEX idx_deal_document_hide_attachment ON deal_document_hide (attachment_id);
CREATE INDEX idx_deal_room_comment_room ON deal_room_comment (room_id);
CREATE INDEX idx_deal_room_engagement_participant_room ON deal_room_engagement (participant_id, room_id);
CREATE INDEX idx_deal_room_session_participant_room ON deal_room_session (participant_id, room_id);
CREATE INDEX idx_deal_room_session_room ON deal_room_session (room_id);
CREATE INDEX idx_dedupe_candidate_left_company ON dedupe_candidate (left_company_id);
CREATE INDEX idx_dedupe_candidate_left_contact ON dedupe_candidate (left_contact_id);
CREATE INDEX idx_dedupe_candidate_left_lead ON dedupe_candidate (left_lead_id);
CREATE INDEX idx_dedupe_candidate_right_company ON dedupe_candidate (right_company_id);
CREATE INDEX idx_dedupe_candidate_right_contact ON dedupe_candidate (right_contact_id);
CREATE INDEX idx_dedupe_candidate_right_lead ON dedupe_candidate (right_lead_id);
CREATE INDEX idx_linkedin_connection_matched_contact ON linkedin_connection (matched_contact_id);
CREATE INDEX idx_oauth_authorization_code_user ON oauth_authorization_code (user_id);
CREATE INDEX idx_privacy_notice_case_contact ON privacy_notice_case (contact_id);
CREATE INDEX idx_provider_applied_field_run_contact_provider ON provider_applied_field (run_id, contact_id, provider);
CREATE INDEX idx_role_assignment_team ON role_assignment (team_id);
CREATE INDEX idx_stage_progression_outcome_from_stage ON stage_progression_outcome (from_stage_id);
CREATE INDEX idx_stage_progression_outcome_to_stage ON stage_progression_outcome (to_stage_id);
CREATE INDEX idx_stage_progression_policy_from_stage ON stage_progression_policy (from_stage_id);
CREATE INDEX idx_stage_progression_policy_to_stage ON stage_progression_policy (to_stage_id);
CREATE INDEX idx_webhook_subscription_owner ON webhook_subscription (owner_id);
CREATE INDEX idx_withdrawal_credential_purpose ON withdrawal_credential (purpose_id);
