-- A bigint that reaches a JSON client is bounded to what the client can hold.
--
-- Postgres bigint runs from −2^63 to 2^63−1 — asymmetric, and the low end is one
-- further out than the high. A JavaScript number represents integers exactly only to
-- ±(2^53−1), and the contract sends the first as the second: a field declared
-- format: int64 becomes a plain `number` in the generated client. Above the line
-- JSON.parse rounds to the nearest double, silently — no error, nothing in the types
-- marking the boundary, and the browser renders a different number than this
-- database holds.
--
-- Today's values do not reach 2^53. That is a property of the data rather than of the
-- mapping, and nothing stated it until now. The margin is thinner than the round
-- number suggests: the zero-decimal currencies in currency_minor_digits cost two
-- orders of magnitude, so the ceiling moved 100x because of a row in a lookup table.
--
-- 157 columns, derived from the catalog and crm.yaml rather than listed by hand;
-- gates/jsonsafeinteger_test.go holds the rule for the next one.
--
-- NOT VALID here and VALIDATE in the file beside this one. A plain ADD CONSTRAINT
-- scans every row of every one of these tables while holding ACCESS EXCLUSIVE, and
-- this runner applies a file in one transaction, so pairing the two here would hold
-- that lock across both passes.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity ADD CONSTRAINT activity_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE ai_call ADD CONSTRAINT ai_call_cache_write_tokens_js_safe
    CHECK (cache_write_tokens BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE ai_call ADD CONSTRAINT ai_call_cached_tokens_js_safe
    CHECK (cached_tokens BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE ai_call ADD CONSTRAINT ai_call_latency_ms_js_safe
    CHECK (latency_ms BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE ai_call ADD CONSTRAINT ai_call_reasoning_tokens_js_safe
    CHECK (reasoning_tokens BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE ai_call ADD CONSTRAINT ai_call_tokens_in_js_safe
    CHECK (tokens_in BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE ai_call ADD CONSTRAINT ai_call_tokens_out_js_safe
    CHECK (tokens_out BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE ai_feedback ADD CONSTRAINT ai_feedback_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE ai_usage ADD CONSTRAINT ai_usage_cache_write_tokens_js_safe
    CHECK (cache_write_tokens BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE ai_usage ADD CONSTRAINT ai_usage_cached_tokens_js_safe
    CHECK (cached_tokens BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE ai_usage ADD CONSTRAINT ai_usage_calls_js_safe
    CHECK (calls BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE ai_usage ADD CONSTRAINT ai_usage_reasoning_tokens_js_safe
    CHECK (reasoning_tokens BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE ai_usage ADD CONSTRAINT ai_usage_tokens_in_js_safe
    CHECK (tokens_in BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE ai_usage ADD CONSTRAINT ai_usage_tokens_out_js_safe
    CHECK (tokens_out BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE analytics_share ADD CONSTRAINT analytics_share_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE approval ADD CONSTRAINT approval_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE assurance_exception ADD CONSTRAINT assurance_exception_affected_minor_js_safe
    CHECK (affected_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE assurance_exception ADD CONSTRAINT assurance_exception_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE assurance_resolution ADD CONSTRAINT assurance_resolution_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE assurance_run ADD CONSTRAINT assurance_run_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE assurance_source_coverage ADD CONSTRAINT assurance_source_coverage_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE attachment ADD CONSTRAINT attachment_byte_size_js_safe
    CHECK (byte_size BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE automation ADD CONSTRAINT automation_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE brief_run ADD CONSTRAINT brief_run_revenue_norm_minor_js_safe
    CHECK (revenue_norm_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE channel_connection ADD CONSTRAINT channel_connection_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE close_date_run ADD CONSTRAINT close_date_run_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE commission_entry ADD CONSTRAINT commission_entry_amount_minor_js_safe
    CHECK (amount_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE commission_entry ADD CONSTRAINT commission_entry_basis_amount_minor_js_safe
    CHECK (basis_amount_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE commission_entry ADD CONSTRAINT commission_entry_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE company ADD CONSTRAINT company_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE company_domain ADD CONSTRAINT company_domain_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE company_fact ADD CONSTRAINT company_fact_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE company_profile_field ADD CONSTRAINT company_profile_field_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE company_relationship_type ADD CONSTRAINT company_relationship_type_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE company_vat_check ADD CONSTRAINT company_vat_check_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE contact ADD CONSTRAINT contact_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE contact_channel_identity ADD CONSTRAINT contact_channel_identity_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE contact_email ADD CONSTRAINT contact_email_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE contact_phone ADD CONSTRAINT contact_phone_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE contact_profile_field ADD CONSTRAINT contact_profile_field_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE contract ADD CONSTRAINT contract_arr_minor_js_safe
    CHECK (arr_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE contract ADD CONSTRAINT contract_value_minor_js_safe
    CHECK (value_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE contract ADD CONSTRAINT contract_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE conversation_claim ADD CONSTRAINT conversation_claim_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE custom_field ADD CONSTRAINT custom_field_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE deal ADD CONSTRAINT deal_amount_minor_js_safe
    CHECK (amount_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE deal ADD CONSTRAINT deal_expected_arr_minor_js_safe
    CHECK (expected_arr_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE deal ADD CONSTRAINT deal_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE deal_acquisition_source ADD CONSTRAINT deal_acquisition_source_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE deal_correction ADD CONSTRAINT deal_correction_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE deal_room ADD CONSTRAINT deal_room_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE deal_room_document ADD CONSTRAINT deal_room_document_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE deal_room_thread ADD CONSTRAINT deal_room_thread_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE deal_stage_evidence ADD CONSTRAINT deal_stage_evidence_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE deal_suggestion ADD CONSTRAINT deal_suggestion_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE dedupe_candidate ADD CONSTRAINT dedupe_candidate_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE email_signature ADD CONSTRAINT email_signature_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE finance_connection ADD CONSTRAINT finance_connection_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE finance_customer_link ADD CONSTRAINT finance_customer_link_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE finance_external_customer ADD CONSTRAINT finance_external_customer_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE finance_invoice ADD CONSTRAINT finance_invoice_gross_minor_js_safe
    CHECK (gross_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE finance_invoice ADD CONSTRAINT finance_invoice_net_minor_js_safe
    CHECK (net_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE finance_invoice ADD CONSTRAINT finance_invoice_open_minor_js_safe
    CHECK (open_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE finance_invoice ADD CONSTRAINT finance_invoice_tax_minor_js_safe
    CHECK (tax_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE finance_invoice ADD CONSTRAINT finance_invoice_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE finance_payment ADD CONSTRAINT finance_payment_amount_minor_js_safe
    CHECK (amount_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE finance_payment ADD CONSTRAINT finance_payment_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE forecast_call ADD CONSTRAINT forecast_call_amount_minor_js_safe
    CHECK (amount_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE forecast_call ADD CONSTRAINT forecast_call_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE forecast_contribution ADD CONSTRAINT forecast_contribution_amount_minor_js_safe
    CHECK (amount_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE forecast_contribution ADD CONSTRAINT forecast_contribution_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE forecast_contribution ADD CONSTRAINT forecast_contribution_weighted_minor_js_safe
    CHECK (weighted_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE forecast_snapshot ADD CONSTRAINT forecast_snapshot_best_case_minor_js_safe
    CHECK (best_case_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE forecast_snapshot ADD CONSTRAINT forecast_snapshot_evidence_minor_js_safe
    CHECK (evidence_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE forecast_snapshot ADD CONSTRAINT forecast_snapshot_open_minor_js_safe
    CHECK (open_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE forecast_snapshot ADD CONSTRAINT forecast_snapshot_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE forecast_snapshot ADD CONSTRAINT forecast_snapshot_weighted_minor_js_safe
    CHECK (weighted_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE forecast_snapshot ADD CONSTRAINT forecast_snapshot_won_minor_js_safe
    CHECK (won_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE knowledge_document ADD CONSTRAINT knowledge_document_byte_size_js_safe
    CHECK (byte_size BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE lead ADD CONSTRAINT lead_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE lead_disqualify_reason ADD CONSTRAINT lead_disqualify_reason_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE lead_source ADD CONSTRAINT lead_source_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE list ADD CONSTRAINT list_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE list_evaluation ADD CONSTRAINT list_evaluation_definition_version_js_safe
    CHECK (definition_version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE list_live_member ADD CONSTRAINT list_live_member_definition_version_js_safe
    CHECK (definition_version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE list_member_event ADD CONSTRAINT list_member_event_definition_version_js_safe
    CHECK (definition_version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE list_revision ADD CONSTRAINT list_revision_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE mail_draft ADD CONSTRAINT mail_draft_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE meeting_invitation ADD CONSTRAINT meeting_invitation_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE offer ADD CONSTRAINT offer_gross_minor_js_safe
    CHECK (gross_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE offer ADD CONSTRAINT offer_net_minor_js_safe
    CHECK (net_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE offer ADD CONSTRAINT offer_tax_minor_js_safe
    CHECK (tax_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE offer ADD CONSTRAINT offer_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE offer_line_item ADD CONSTRAINT offer_line_item_unit_price_minor_js_safe
    CHECK (unit_price_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE offer_line_item ADD CONSTRAINT offer_line_item_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE offer_template ADD CONSTRAINT offer_template_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE onboarding_wizard_state ADD CONSTRAINT onboarding_wizard_state_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE partner ADD CONSTRAINT partner_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE pipeline ADD CONSTRAINT pipeline_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE product ADD CONSTRAINT product_unit_price_minor_js_safe
    CHECK (unit_price_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE product ADD CONSTRAINT product_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE project ADD CONSTRAINT project_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE provider_connection ADD CONSTRAINT provider_connection_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE provider_run ADD CONSTRAINT provider_run_connection_version_js_safe
    CHECK (connection_version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE record_assignment ADD CONSTRAINT record_assignment_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE record_grant ADD CONSTRAINT record_grant_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE record_role ADD CONSTRAINT record_role_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE relationship ADD CONSTRAINT relationship_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE report_definition ADD CONSTRAINT report_definition_revision_js_safe
    CHECK (revision BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE report_definition ADD CONSTRAINT report_definition_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE report_definition_revision ADD CONSTRAINT report_definition_revision_framework_revision_js_safe
    CHECK (framework_revision BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE report_definition_revision ADD CONSTRAINT report_definition_revision_revision_js_safe
    CHECK (revision BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE report_edition ADD CONSTRAINT report_edition_report_revision_js_safe
    CHECK (report_revision BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE report_execution ADD CONSTRAINT report_execution_attempt_js_safe
    CHECK (attempt BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE report_execution ADD CONSTRAINT report_execution_report_revision_js_safe
    CHECK (report_revision BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE report_schedule ADD CONSTRAINT report_schedule_report_revision_js_safe
    CHECK (report_revision BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE report_schedule ADD CONSTRAINT report_schedule_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE reporting_framework ADD CONSTRAINT reporting_framework_revision_js_safe
    CHECK (revision BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE reporting_framework ADD CONSTRAINT reporting_framework_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE reporting_framework_revision ADD CONSTRAINT reporting_framework_revision_revision_js_safe
    CHECK (revision BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE role ADD CONSTRAINT role_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE sales_target ADD CONSTRAINT sales_target_revision_js_safe
    CHECK (revision BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE sales_target ADD CONSTRAINT sales_target_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE sales_target_revision ADD CONSTRAINT sales_target_revision_revision_js_safe
    CHECK (revision BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE saved_view ADD CONSTRAINT saved_view_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE scheduled_send ADD CONSTRAINT scheduled_send_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE sdr_handoff ADD CONSTRAINT sdr_handoff_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE sdr_handoff_reason ADD CONSTRAINT sdr_handoff_reason_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE signal ADD CONSTRAINT signal_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE stage ADD CONSTRAINT stage_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE stage_exit_criterion ADD CONSTRAINT stage_exit_criterion_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE stage_progression_outcome ADD CONSTRAINT stage_progression_outcome_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE stage_progression_policy ADD CONSTRAINT stage_progression_policy_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE tag ADD CONSTRAINT tag_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE team_weekly_review_outlook ADD CONSTRAINT team_weekly_review_outlook_best_case_minor_js_safe
    CHECK (best_case_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE team_weekly_review_outlook ADD CONSTRAINT team_weekly_review_outlook_closing_landing_minor_js_safe
    CHECK (closing_landing_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE team_weekly_review_outlook ADD CONSTRAINT team_weekly_review_outlook_commit_minor_js_safe
    CHECK (commit_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE team_weekly_review_outlook ADD CONSTRAINT team_weekly_review_outlook_opening_landing_minor_js_safe
    CHECK (opening_landing_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE team_weekly_review_outlook ADD CONSTRAINT team_weekly_review_outlook_weighted_minor_js_safe
    CHECK (weighted_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE team_weekly_review_outlook ADD CONSTRAINT team_weekly_review_outlook_won_minor_js_safe
    CHECK (won_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE voice_build ADD CONSTRAINT voice_build_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE voice_corpus_source ADD CONSTRAINT voice_corpus_source_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE voice_learning_signal ADD CONSTRAINT voice_learning_signal_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE voice_profile ADD CONSTRAINT voice_profile_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE voice_profile_version ADD CONSTRAINT voice_profile_version_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE webhook_subscription ADD CONSTRAINT webhook_subscription_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE weekly_plan ADD CONSTRAINT weekly_plan_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE weekly_plan_commitment ADD CONSTRAINT weekly_plan_commitment_version_js_safe
    CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE weekly_review_deal ADD CONSTRAINT weekly_review_deal_amount_minor_at_close_js_safe
    CHECK (amount_minor_at_close BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE weekly_review_driver ADD CONSTRAINT weekly_review_driver_delta_minor_js_safe
    CHECK (delta_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE weekly_review_movement ADD CONSTRAINT weekly_review_movement_delta_minor_js_safe
    CHECK (delta_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE weekly_review_outlook ADD CONSTRAINT weekly_review_outlook_best_case_minor_js_safe
    CHECK (best_case_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE weekly_review_outlook ADD CONSTRAINT weekly_review_outlook_closing_landing_minor_js_safe
    CHECK (closing_landing_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE weekly_review_outlook ADD CONSTRAINT weekly_review_outlook_commit_minor_js_safe
    CHECK (commit_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE weekly_review_outlook ADD CONSTRAINT weekly_review_outlook_opening_landing_minor_js_safe
    CHECK (opening_landing_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE weekly_review_outlook ADD CONSTRAINT weekly_review_outlook_weighted_minor_js_safe
    CHECK (weighted_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
ALTER TABLE weekly_review_outlook ADD CONSTRAINT weekly_review_outlook_won_minor_js_safe
    CHECK (won_minor BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;
