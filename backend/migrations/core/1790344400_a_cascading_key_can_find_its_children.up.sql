-- An index on every ON DELETE CASCADE foreign key's referencing columns.
--
-- Postgres has to FIND the child rows before it can cascade a delete. With no
-- index on the referencing column that is a sequential scan of the child table,
-- performed inside the parent's delete transaction and holding its locks.
--
-- Two places this is not theoretical. `dedupe_candidate` cascades from six
-- directions -- contact, company and lead, each on a left and a right column --
-- so deleting any one of those record types scans the whole dedupe queue six
-- times in one transaction. And Article 17 erasure deletes contacts by design,
-- running against dedupe_candidate twice, privacy_notice_case, contact_email,
-- contact_phone, preference_token, relationship and half a dozen more at once.
--
-- ONLY the cascading keys. The schema has far more unindexed foreign keys than
-- these, and most of them are correct as they are: the column exists for
-- integrity, nothing asks the reverse question, and an index nobody reads is
-- pure write cost. A cascade is different because the DELETE itself is the
-- reader.
--
-- A PARTIAL index does not answer a cascade. `WHERE archived_at IS NULL` leaves
-- out exactly the rows an erasure still has to remove, so the planner cannot use
-- it and falls back to the scan. Where the partial covered the same columns it
-- is widened here rather than joined by a second index: the full one answers
-- every query the partial did. Where it carries extra trailing columns a
-- narrower `_cascade` index is added beside it instead.
-- `WHERE <fk column> IS NOT NULL` is the exception -- `col = $1` implies it, so
-- those indexes already serve and are left alone.
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

-- CREATE INDEX takes a SHARE lock, and DROP INDEX an ACCESS EXCLUSIVE one, both
-- blocking writers on tables this migration did not create. Bounded: failing
-- fast is recoverable, and stalling every write to dedupe_candidate behind one
-- open transaction is not.
SET LOCAL lock_timeout = '3s';
DROP INDEX idx_brief_item_deal;
DROP INDEX idx_company_domain_company;
DROP INDEX idx_contact_email_contact;
DROP INDEX idx_contact_phone_contact;
DROP INDEX idx_record_assignment_company;
DROP INDEX idx_record_assignment_deal;
DROP INDEX idx_record_assignment_project;
DROP INDEX idx_session_user;
DROP INDEX idx_stage_pipeline;
CREATE INDEX idx_activity_request_settlement_judged_through_activity ON activity_request_settlement (judged_through_activity_id);
CREATE INDEX idx_analytics_share_snapshot ON analytics_share (snapshot_id);
CREATE INDEX idx_auth_token_user_cascade ON auth_token (user_id);
CREATE INDEX idx_booking_page_host_user ON booking_page (host_user_id);
CREATE INDEX idx_brief_item_deal ON brief_item (deal_id);
CREATE INDEX idx_capture_exclusion_user ON capture_exclusion (user_id);
CREATE INDEX idx_capture_pending_counterparty_activity ON capture_pending_counterparty (activity_id);
CREATE INDEX idx_capture_pending_counterparty_owner ON capture_pending_counterparty (owner_id);
CREATE INDEX idx_close_date_run_member_deal ON close_date_run_member (deal_id);
CREATE INDEX idx_communication_basis_contact ON communication_basis (contact_id);
CREATE INDEX idx_communication_basis_lead ON communication_basis (lead_id);
CREATE INDEX idx_communication_decision_delivery ON communication_decision (delivery_id);
CREATE INDEX idx_communication_review_delivery_intent ON communication_review (delivery_intent_id);
CREATE INDEX idx_communication_suppression_contact ON communication_suppression (contact_id);
CREATE INDEX idx_communication_suppression_lead ON communication_suppression (lead_id);
CREATE INDEX idx_company_domain_company ON company_domain (company_id);
CREATE INDEX idx_company_domain_disposition_company ON company_domain_disposition (company_id);
CREATE INDEX idx_contact_confirm_submission_token ON contact_confirm_submission (token_id);
CREATE INDEX idx_contact_email_contact ON contact_email (contact_id);
CREATE INDEX idx_contact_phone_contact ON contact_phone (contact_id);
CREATE INDEX idx_contact_signature_enrich_state_activity ON contact_signature_enrich_state (activity_id);
CREATE INDEX idx_conversation_claim_contact ON conversation_claim (contact_id);
CREATE INDEX idx_conversation_claim_source_activity ON conversation_claim (source_activity_id);
CREATE INDEX idx_deal_correction_deal ON deal_correction (deal_id);
CREATE INDEX idx_deal_document_hide_attachment ON deal_document_hide (attachment_id);
CREATE INDEX idx_deal_room_deal ON deal_room (deal_id);
CREATE INDEX idx_deal_room_comment_room ON deal_room_comment (room_id);
CREATE INDEX idx_deal_room_document_room_cascade ON deal_room_document (room_id);
CREATE INDEX idx_deal_room_engagement_participant_room ON deal_room_engagement (participant_id, room_id);
CREATE INDEX idx_deal_room_participant_room ON deal_room_participant (room_id);
CREATE INDEX idx_deal_room_session_participant_room ON deal_room_session (participant_id, room_id);
CREATE INDEX idx_deal_room_session_room ON deal_room_session (room_id);
CREATE INDEX idx_deal_room_thread_document ON deal_room_thread (document_id);
CREATE INDEX idx_dedupe_candidate_left_company ON dedupe_candidate (left_company_id);
CREATE INDEX idx_dedupe_candidate_left_contact ON dedupe_candidate (left_contact_id);
CREATE INDEX idx_dedupe_candidate_left_lead ON dedupe_candidate (left_lead_id);
CREATE INDEX idx_dedupe_candidate_right_company ON dedupe_candidate (right_company_id);
CREATE INDEX idx_dedupe_candidate_right_contact ON dedupe_candidate (right_contact_id);
CREATE INDEX idx_dedupe_candidate_right_lead ON dedupe_candidate (right_lead_id);
CREATE INDEX idx_linkedin_connection_matched_contact ON linkedin_connection (matched_contact_id);
CREATE INDEX idx_linkedin_connection_owner_user ON linkedin_connection (owner_user_id);
CREATE INDEX idx_notice_recipient_user ON notice (recipient_user_id);
CREATE INDEX idx_oauth_authorization_code_user ON oauth_authorization_code (user_id);
CREATE INDEX idx_oauth_grant_user ON oauth_grant (user_id);
CREATE INDEX idx_passport_on_behalf_of ON passport (on_behalf_of);
CREATE INDEX idx_preference_token_contact ON preference_token (contact_id);
CREATE INDEX idx_privacy_notice_case_contact ON privacy_notice_case (contact_id);
CREATE INDEX idx_provider_applied_field_run_contact_provider ON provider_applied_field (run_id, contact_id, provider);
CREATE INDEX idx_provider_run_contact ON provider_run (contact_id);
CREATE INDEX idx_record_assignment_company ON record_assignment (company_id);
CREATE INDEX idx_record_assignment_deal ON record_assignment (deal_id);
CREATE INDEX idx_record_assignment_project ON record_assignment (project_id);
CREATE INDEX idx_relationship_counterparty_contact ON relationship (counterparty_contact_id);
CREATE INDEX idx_role_assignment_team ON role_assignment (team_id);
CREATE INDEX idx_saved_view_owner_cascade ON saved_view (owner_id);
CREATE INDEX idx_session_user ON session (user_id);
CREATE INDEX idx_stage_pipeline ON stage (pipeline_id);
CREATE INDEX idx_stage_progression_outcome_from_stage ON stage_progression_outcome (from_stage_id);
CREATE INDEX idx_stage_progression_outcome_to_stage ON stage_progression_outcome (to_stage_id);
CREATE INDEX idx_stage_progression_policy_from_stage ON stage_progression_policy (from_stage_id);
CREATE INDEX idx_stage_progression_policy_to_stage ON stage_progression_policy (to_stage_id);
CREATE INDEX idx_webhook_subscription_owner ON webhook_subscription (owner_id);
CREATE INDEX idx_withdrawal_credential_purpose ON withdrawal_credential (purpose_id);
