-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"agenda_fingerprint":20000,"audience":20000,"audience_reason":2000,"body_completeness":20000,"calendar_method":20000,"calendar_uid":20000,"call_outcome":20000,"capture_label":120,"capture_label_declined_ruleset":20000,"captured_by":20000,"channel_provider":20000,"counterparty_email":20000,"deal_topic_digest":20000,"direction":20000,"due_zone":20000,"kind":20000,"language":20000,"meeting_status":20000,"meeting_zone":20000,"occurred_precision":20000,"origin":20000,"owed_verdict":20000,"owed_verdict_declined_ruleset":20000,"owed_verdict_ruleset":20000,"redacted_fields":20000,"remind_zone":20000,"reply_verdict":20000,"reply_verdict_by":20000,"restricted_reason":2000,"retention_class":20000,"source":20000,"source_id":20000,"source_system":20000,"subject":20000,"task_type":20000,"thread_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"agenda_fingerprint":20000,"audience":20000,"audience_reason":2000,"body_completeness":20000,"calendar_method":20000,"calendar_uid":20000,"call_outcome":20000,"capture_label":120,"capture_label_declined_ruleset":20000,"captured_by":20000,"channel_provider":20000,"counterparty_email":20000,"deal_topic_digest":20000,"direction":20000,"due_zone":20000,"kind":20000,"language":20000,"meeting_status":20000,"meeting_zone":20000,"occurred_precision":20000,"origin":20000,"owed_verdict":20000,"owed_verdict_declined_ruleset":20000,"owed_verdict_ruleset":20000,"redacted_fields":20000,"remind_zone":20000,"reply_verdict":20000,"reply_verdict_by":20000,"restricted_reason":2000,"retention_class":20000,"source":20000,"source_id":20000,"source_system":20000,"subject":20000,"task_type":20000,"thread_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"agenda_fingerprint":20000,"audience":20000,"audience_reason":2000,"body_completeness":20000,"calendar_method":20000,"calendar_uid":20000,"call_outcome":20000,"capture_label":120,"capture_label_declined_ruleset":20000,"captured_by":20000,"channel_provider":20000,"counterparty_email":20000,"deal_topic_digest":20000,"direction":20000,"due_zone":20000,"kind":20000,"language":20000,"meeting_status":20000,"meeting_zone":20000,"occurred_precision":20000,"origin":20000,"owed_verdict":20000,"owed_verdict_declined_ruleset":20000,"owed_verdict_ruleset":20000,"redacted_fields":20000,"remind_zone":20000,"reply_verdict":20000,"reply_verdict_by":20000,"restricted_reason":2000,"retention_class":20000,"source":20000,"source_id":20000,"source_system":20000,"subject":20000,"task_type":20000,"thread_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"agenda_fingerprint":20000,"audience":20000,"audience_reason":2000,"body_completeness":20000,"calendar_method":20000,"calendar_uid":20000,"call_outcome":20000,"capture_label":120,"capture_label_declined_ruleset":20000,"captured_by":20000,"channel_provider":20000,"counterparty_email":20000,"deal_topic_digest":20000,"direction":20000,"due_zone":20000,"kind":20000,"language":20000,"meeting_status":20000,"meeting_zone":20000,"occurred_precision":20000,"origin":20000,"owed_verdict":20000,"owed_verdict_declined_ruleset":20000,"owed_verdict_ruleset":20000,"redacted_fields":20000,"remind_zone":20000,"reply_verdict":20000,"reply_verdict_by":20000,"restricted_reason":2000,"retention_class":20000,"source":20000,"source_id":20000,"source_system":20000,"subject":20000,"task_type":20000,"thread_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_audience_member" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"created_by":20000,"subject_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"created_by":20000,"subject_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_audience_member" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"created_by":20000,"subject_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"created_by":20000,"subject_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_calendar_method_backfill" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"direction":20000,"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"direction":20000,"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_calendar_method_backfill" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"direction":20000,"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"direction":20000,"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_commitment_admission" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"locator":20000,"outcome":20000,"rule_version":20000,"source_quote":20000,"summary":500}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"locator":20000,"outcome":20000,"rule_version":20000,"source_quote":20000,"summary":500}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_commitment_admission" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"locator":20000,"outcome":20000,"rule_version":20000,"source_quote":20000,"summary":500}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"locator":20000,"outcome":20000,"rule_version":20000,"source_quote":20000,"summary":500}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_identity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"attested_by":20000,"identity_key":20000,"identity_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"attested_by":20000,"identity_key":20000,"identity_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_identity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"attested_by":20000,"identity_key":20000,"identity_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"attested_by":20000,"identity_key":20000,"identity_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_kind" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_kind" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_link" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"entity_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"entity_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_link" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"entity_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"entity_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_link_suggestion" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"decided_by":20000,"entity_type":20000,"reason":500,"status":20000,"unit":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"decided_by":20000,"entity_type":20000,"reason":500,"status":20000,"unit":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_link_suggestion" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"decided_by":20000,"entity_type":20000,"reason":500,"status":20000,"unit":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"decided_by":20000,"entity_type":20000,"reason":500,"status":20000,"unit":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_mail_reference" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"referenced_id":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"referenced_id":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_mail_reference" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"referenced_id":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"referenced_id":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_meeting_attendee_repair" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_meeting_attendee_repair" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_meeting_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"actor":20000,"scheduled_precision":20000,"source_id":20000,"source_system":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"actor":20000,"scheduled_precision":20000,"source_id":20000,"source_system":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_meeting_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"actor":20000,"scheduled_precision":20000,"source_id":20000,"source_system":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"actor":20000,"scheduled_precision":20000,"source_id":20000,"source_system":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_meeting_rsvp_backfill" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_meeting_rsvp_backfill" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_own_sent_mail_repair" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_own_sent_mail_repair" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_participant" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"address":20000,"address_domain_folded":20000,"channel_user_id":20000,"domain_evidence_precision":20000,"response":20000,"role":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address":20000,"address_domain_folded":20000,"channel_user_id":20000,"domain_evidence_precision":20000,"response":20000,"role":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_participant" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"address":20000,"address_domain_folded":20000,"channel_user_id":20000,"domain_evidence_precision":20000,"response":20000,"role":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address":20000,"address_domain_folded":20000,"channel_user_id":20000,"domain_evidence_precision":20000,"response":20000,"role":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_participant_replay" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_participant_replay" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_reader_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"reopen_on":20000,"set_by":20000,"snoozed_zone":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"reopen_on":20000,"set_by":20000,"snoozed_zone":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_reader_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"reopen_on":20000,"set_by":20000,"snoozed_zone":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"reopen_on":20000,"set_by":20000,"snoozed_zone":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_reply_verdict_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"decided_by":20000,"verdict":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"decided_by":20000,"verdict":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_reply_verdict_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"decided_by":20000,"verdict":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"decided_by":20000,"verdict":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_request_settlement" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"decided_by":20000,"judged_evidence":20000,"remaining":20000,"rule_version":20000,"verdict":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"decided_by":20000,"judged_evidence":20000,"remaining":20000,"rule_version":20000,"verdict":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_request_settlement" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"decided_by":20000,"judged_evidence":20000,"remaining":20000,"rule_version":20000,"verdict":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"decided_by":20000,"judged_evidence":20000,"remaining":20000,"rule_version":20000,"verdict":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_retention_evidence" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"basis":20000,"deal_name":120,"decided_by_name":120,"project_name":120,"reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"basis":20000,"deal_name":120,"decided_by_name":120,"project_name":120,"reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_retention_evidence" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"basis":20000,"deal_name":120,"decided_by_name":120,"project_name":120,"reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"basis":20000,"deal_name":120,"decided_by_name":120,"project_name":120,"reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_review_response" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"outcome":20000,"source":20000,"template_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"outcome":20000,"source":20000,"template_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_review_response" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"outcome":20000,"source":20000,"template_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"outcome":20000,"source":20000,"template_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_review_template" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"key":20000,"label":120,"outcome":20000,"shipped_label":120,"shipped_label_key":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"key":20000,"label":120,"outcome":20000,"shipped_label":120,"shipped_label_key":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_review_template" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"key":20000,"label":120,"outcome":20000,"shipped_label":120,"shipped_label_key":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"key":20000,"label":120,"outcome":20000,"shipped_label":120,"shipped_label_key":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_sales_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"channel_provider":20000,"kind":20000,"set_by":20000,"thread_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"channel_provider":20000,"kind":20000,"set_by":20000,"thread_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_sales_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"channel_provider":20000,"kind":20000,"set_by":20000,"thread_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"channel_provider":20000,"kind":20000,"set_by":20000,"thread_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."agent_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"agent_spec":20000,"degrade_reason":2000,"goal":20000,"status":20000,"trigger_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"agent_spec":20000,"degrade_reason":2000,"goal":20000,"status":20000,"trigger_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."agent_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"agent_spec":20000,"degrade_reason":2000,"goal":20000,"status":20000,"trigger_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"agent_spec":20000,"degrade_reason":2000,"goal":20000,"status":20000,"trigger_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."agent_standing_grant" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"agent_spec":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"agent_spec":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."agent_standing_grant" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"agent_spec":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"agent_spec":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."agent_task" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"status":20000,"status_message":20000,"tool":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"status":20000,"status_message":20000,"tool":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."agent_task" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"status":20000,"status_message":20000,"tool":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"status":20000,"status_message":20000,"tool":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."ai_call" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"attempt_reason":2000,"config_hash":20000,"context_fingerprint":20000,"context_scopes":20000,"decision_choice":20000,"error_sentinel":20000,"finish_reason":2000,"kind":20000,"model_id":20000,"provider":20000,"request_fingerprint":20000,"schema_downgrade":20000,"secret_kinds":20000,"served_identity_source":20000,"served_model":20000,"served_provider":20000,"subject_type":20000,"task":20000,"tier":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"attempt_reason":2000,"config_hash":20000,"context_fingerprint":20000,"context_scopes":20000,"decision_choice":20000,"error_sentinel":20000,"finish_reason":2000,"kind":20000,"model_id":20000,"provider":20000,"request_fingerprint":20000,"schema_downgrade":20000,"secret_kinds":20000,"served_identity_source":20000,"served_model":20000,"served_provider":20000,"subject_type":20000,"task":20000,"tier":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."ai_call" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"attempt_reason":2000,"config_hash":20000,"context_fingerprint":20000,"context_scopes":20000,"decision_choice":20000,"error_sentinel":20000,"finish_reason":2000,"kind":20000,"model_id":20000,"provider":20000,"request_fingerprint":20000,"schema_downgrade":20000,"secret_kinds":20000,"served_identity_source":20000,"served_model":20000,"served_provider":20000,"subject_type":20000,"task":20000,"tier":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"attempt_reason":2000,"config_hash":20000,"context_fingerprint":20000,"context_scopes":20000,"decision_choice":20000,"error_sentinel":20000,"finish_reason":2000,"kind":20000,"model_id":20000,"provider":20000,"request_fingerprint":20000,"schema_downgrade":20000,"secret_kinds":20000,"served_identity_source":20000,"served_model":20000,"served_provider":20000,"subject_type":20000,"task":20000,"tier":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."ai_call_config" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"hash":20000,"prompt_version":20000,"routing_config_hash":20000,"task_contract_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"hash":20000,"prompt_version":20000,"routing_config_hash":20000,"task_contract_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."ai_call_config" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"hash":20000,"prompt_version":20000,"routing_config_hash":20000,"task_contract_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"hash":20000,"prompt_version":20000,"routing_config_hash":20000,"task_contract_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."ai_feedback" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"claim_key":20000,"claim_kind":20000,"corrected_value":20000,"note":2000,"source":20000,"subject_type":20000,"value_shown":20000,"verdict":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"claim_key":20000,"claim_kind":20000,"corrected_value":20000,"note":2000,"source":20000,"subject_type":20000,"value_shown":20000,"verdict":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."ai_feedback" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"claim_key":20000,"claim_kind":20000,"corrected_value":20000,"note":2000,"source":20000,"subject_type":20000,"value_shown":20000,"verdict":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"claim_key":20000,"claim_kind":20000,"corrected_value":20000,"note":2000,"source":20000,"subject_type":20000,"value_shown":20000,"verdict":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."ai_model_rate" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"lane":20000,"model_id":20000,"provider":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"lane":20000,"model_id":20000,"provider":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."ai_model_rate" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"lane":20000,"model_id":20000,"provider":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"lane":20000,"model_id":20000,"provider":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."ai_task_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"actor_scope":20000,"ai_task":20000,"degrade_reason":2000,"kind":20000,"occurrence_key":20000,"quantity_unit":20000,"source":20000,"state":20000,"subject_label":120,"subject_type":20000,"summary":500}')) EXECUTE FUNCTION public.refuse_oversized_text('{"actor_scope":20000,"ai_task":20000,"degrade_reason":2000,"kind":20000,"occurrence_key":20000,"quantity_unit":20000,"source":20000,"state":20000,"subject_label":120,"subject_type":20000,"summary":500}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."ai_task_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"actor_scope":20000,"ai_task":20000,"degrade_reason":2000,"kind":20000,"occurrence_key":20000,"quantity_unit":20000,"source":20000,"state":20000,"subject_label":120,"subject_type":20000,"summary":500}')) EXECUTE FUNCTION public.refuse_oversized_text('{"actor_scope":20000,"ai_task":20000,"degrade_reason":2000,"kind":20000,"occurrence_key":20000,"quantity_unit":20000,"source":20000,"state":20000,"subject_label":120,"subject_type":20000,"summary":500}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."ai_usage" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"task":20000,"tier":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"task":20000,"tier":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."ai_usage" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"task":20000,"tier":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"task":20000,"tier":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."analytics_share" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"kind":20000,"scope_kind":20000,"target":20000,"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"kind":20000,"scope_kind":20000,"target":20000,"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."analytics_share" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"kind":20000,"scope_kind":20000,"target":20000,"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"kind":20000,"scope_kind":20000,"target":20000,"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."app_user" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"avatar_object_key":20000,"avatar_origin":20000,"email":20000,"email_domain_folded":20000,"greeting_name":120,"locale":20000,"morning_brief_delivery":20000,"password_hash":20000,"routine_delivery_time":20000,"routine_mail_scope":20000,"seat_type":20000,"service_extension":20000,"status":20000,"timezone":20000,"timezone_set_by":20000,"weekly_delivery":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"avatar_object_key":20000,"avatar_origin":20000,"email":20000,"email_domain_folded":20000,"greeting_name":120,"locale":20000,"morning_brief_delivery":20000,"password_hash":20000,"routine_delivery_time":20000,"routine_mail_scope":20000,"seat_type":20000,"service_extension":20000,"status":20000,"timezone":20000,"timezone_set_by":20000,"weekly_delivery":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."app_user" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"avatar_object_key":20000,"avatar_origin":20000,"email":20000,"email_domain_folded":20000,"greeting_name":120,"locale":20000,"morning_brief_delivery":20000,"password_hash":20000,"routine_delivery_time":20000,"routine_mail_scope":20000,"seat_type":20000,"service_extension":20000,"status":20000,"timezone":20000,"timezone_set_by":20000,"weekly_delivery":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"avatar_object_key":20000,"avatar_origin":20000,"email":20000,"email_domain_folded":20000,"greeting_name":120,"locale":20000,"morning_brief_delivery":20000,"password_hash":20000,"routine_delivery_time":20000,"routine_mail_scope":20000,"seat_type":20000,"service_extension":20000,"status":20000,"timezone":20000,"timezone_set_by":20000,"weekly_delivery":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."approval" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"co_target_entity_type":20000,"context_digest":20000,"decision_reason":2000,"diff_hash":20000,"effect_failure":20000,"kind":20000,"proposed_by":20000,"requested_decider_source":20000,"status":20000,"summary":500,"target_entity_type":20000,"target_label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"co_target_entity_type":20000,"context_digest":20000,"decision_reason":2000,"diff_hash":20000,"effect_failure":20000,"kind":20000,"proposed_by":20000,"requested_decider_source":20000,"status":20000,"summary":500,"target_entity_type":20000,"target_label":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."approval" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"co_target_entity_type":20000,"context_digest":20000,"decision_reason":2000,"diff_hash":20000,"effect_failure":20000,"kind":20000,"proposed_by":20000,"requested_decider_source":20000,"status":20000,"summary":500,"target_entity_type":20000,"target_label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"co_target_entity_type":20000,"context_digest":20000,"decision_reason":2000,"diff_hash":20000,"effect_failure":20000,"kind":20000,"proposed_by":20000,"requested_decider_source":20000,"status":20000,"summary":500,"target_entity_type":20000,"target_label":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."approval_autonomy_policy" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"mode":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"mode":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."approval_autonomy_policy" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"mode":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"mode":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."approval_preference" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."approval_preference" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."assurance_cycle" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"opened_by":20000,"scope":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"opened_by":20000,"scope":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."assurance_cycle" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"opened_by":20000,"scope":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"opened_by":20000,"scope":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."assurance_deal_receipt" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"input_fingerprint":20000,"outcome":20000,"policy_version":20000,"reasons":2000,"verdict_basis_digest":20000,"verdict_input_checksum":20000,"verdict_prompt_version":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"input_fingerprint":20000,"outcome":20000,"policy_version":20000,"reasons":2000,"verdict_basis_digest":20000,"verdict_input_checksum":20000,"verdict_prompt_version":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."assurance_deal_receipt" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"input_fingerprint":20000,"outcome":20000,"policy_version":20000,"reasons":2000,"verdict_basis_digest":20000,"verdict_input_checksum":20000,"verdict_prompt_version":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"input_fingerprint":20000,"outcome":20000,"policy_version":20000,"reasons":2000,"verdict_basis_digest":20000,"verdict_input_checksum":20000,"verdict_prompt_version":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."assurance_exception" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"delivery_state":20000,"logical_key":20000,"severity":20000,"status":20000,"subject_kind":20000,"type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"delivery_state":20000,"logical_key":20000,"severity":20000,"status":20000,"subject_kind":20000,"type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."assurance_exception" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"delivery_state":20000,"logical_key":20000,"severity":20000,"status":20000,"subject_kind":20000,"type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"delivery_state":20000,"logical_key":20000,"severity":20000,"status":20000,"subject_kind":20000,"type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."assurance_input_change" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"subject_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"subject_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."assurance_input_change" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"subject_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"subject_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."assurance_resolution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"evidence_ref":20000,"outcome":20000,"reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"evidence_ref":20000,"outcome":20000,"reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."assurance_resolution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"evidence_ref":20000,"outcome":20000,"reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"evidence_ref":20000,"outcome":20000,"reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."assurance_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"digest":20000,"readiness":20000,"requested_by":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"digest":20000,"readiness":20000,"requested_by":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."assurance_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"digest":20000,"readiness":20000,"requested_by":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"digest":20000,"readiness":20000,"requested_by":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."assurance_source_coverage" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"source":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"source":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."assurance_source_coverage" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"source":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"source":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."assurance_task_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"state":20000,"subject_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"state":20000,"subject_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."assurance_task_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"state":20000,"subject_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"state":20000,"subject_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."attachment" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"category":20000,"checksum":20000,"content_type":20000,"declared_type":20000,"doc_state":20000,"entity_type":20000,"external_part_id":20000,"external_source_id":20000,"filename":20000,"source":20000,"storage_key":20000,"title":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"category":20000,"checksum":20000,"content_type":20000,"declared_type":20000,"doc_state":20000,"entity_type":20000,"external_part_id":20000,"external_source_id":20000,"filename":20000,"source":20000,"storage_key":20000,"title":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."attachment" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"category":20000,"checksum":20000,"content_type":20000,"declared_type":20000,"doc_state":20000,"entity_type":20000,"external_part_id":20000,"external_source_id":20000,"filename":20000,"source":20000,"storage_key":20000,"title":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"category":20000,"checksum":20000,"content_type":20000,"declared_type":20000,"doc_state":20000,"entity_type":20000,"external_part_id":20000,"external_source_id":20000,"filename":20000,"source":20000,"storage_key":20000,"title":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."attachment_extraction" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"question":20000,"requested_by":20000,"status":20000,"status_detail":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"question":20000,"requested_by":20000,"status":20000,"status_detail":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."attachment_extraction" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"question":20000,"requested_by":20000,"status":20000,"status_detail":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"question":20000,"requested_by":20000,"status":20000,"status_detail":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."auth_token" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"purpose":20000,"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"purpose":20000,"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."auth_token" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"purpose":20000,"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"purpose":20000,"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."automation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"key":20000,"name":120,"origin":20000,"paused_reason":2000,"tier":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key":20000,"name":120,"origin":20000,"paused_reason":2000,"tier":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."automation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"key":20000,"name":120,"origin":20000,"paused_reason":2000,"tier":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key":20000,"name":120,"origin":20000,"paused_reason":2000,"tier":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."automation_effect_claim" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"effect_fingerprint":20000,"handler":20000,"occurrence_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"effect_fingerprint":20000,"handler":20000,"occurrence_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."automation_effect_claim" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"effect_fingerprint":20000,"handler":20000,"occurrence_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"effect_fingerprint":20000,"handler":20000,"occurrence_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."booking_alias" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"alias":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"alias":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."booking_alias" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"alias":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"alias":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."booking_page" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"slug":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"slug":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."booking_page" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"slug":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"slug":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."brief_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"finding":500,"reopen_on":20000,"snoozed_zone":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"finding":500,"reopen_on":20000,"snoozed_zone":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."brief_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"finding":500,"reopen_on":20000,"snoozed_zone":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"finding":500,"reopen_on":20000,"snoozed_zone":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."brief_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"factors_omitted":20000,"mail_error":500,"narrative":2000,"revenue_norm_currency":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"factors_omitted":20000,"mail_error":500,"narrative":2000,"revenue_norm_currency":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."brief_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"factors_omitted":20000,"mail_error":500,"narrative":2000,"revenue_norm_currency":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"factors_omitted":20000,"mail_error":500,"narrative":2000,"revenue_norm_currency":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."bulk_confirmation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"requested_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"requested_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."bulk_confirmation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"requested_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"requested_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."bulk_operation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"record_type":20000,"requested_by":20000,"verb":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"record_type":20000,"requested_by":20000,"verb":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."bulk_operation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"record_type":20000,"requested_by":20000,"verb":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"record_type":20000,"requested_by":20000,"verb":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_alias_refused" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"value":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"value":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_alias_refused" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"value":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"value":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_alias_sighting" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"source":20000,"value":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"source":20000,"value":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_alias_sighting" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"source":20000,"value":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"source":20000,"value":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_auto_enrich_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"last_outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"last_outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_auto_enrich_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"last_outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"last_outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_backfill" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"last_error_class":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"last_error_class":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_backfill" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"last_error_class":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"last_error_class":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_backfill_creation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_backfill_creation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"account_label":120,"credential_ref":20000,"history_baseline_set_by":20000,"history_baseline_tz":20000,"mail_posture":20000,"provider":20000,"provider_scopes":20000,"scopes":20000,"status":20000,"watch_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"account_label":120,"credential_ref":20000,"history_baseline_set_by":20000,"history_baseline_tz":20000,"mail_posture":20000,"provider":20000,"provider_scopes":20000,"scopes":20000,"status":20000,"watch_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"account_label":120,"credential_ref":20000,"history_baseline_set_by":20000,"history_baseline_tz":20000,"mail_posture":20000,"provider":20000,"provider_scopes":20000,"scopes":20000,"status":20000,"watch_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"account_label":120,"credential_ref":20000,"history_baseline_set_by":20000,"history_baseline_tz":20000,"mail_posture":20000,"provider":20000,"provider_scopes":20000,"scopes":20000,"status":20000,"watch_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_counterparty_hold" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"created_by":20000,"kind":20000,"value":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"created_by":20000,"kind":20000,"value":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_counterparty_hold" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"created_by":20000,"kind":20000,"value":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"created_by":20000,"kind":20000,"value":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_exclusion" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"created_by":20000,"kind":20000,"scope":20000,"value":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"created_by":20000,"kind":20000,"scope":20000,"value":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_exclusion" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"created_by":20000,"kind":20000,"scope":20000,"value":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"created_by":20000,"kind":20000,"scope":20000,"value":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_freemail_domain" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"domain":20000,"kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"domain":20000,"kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_freemail_domain" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"domain":20000,"kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"domain":20000,"kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_import" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"delivered_by":20000,"posture_at_import":20000,"provider_message_ids":20000,"verdict_reason":2000,"verdict_reasons":2000,"verdict_status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"delivered_by":20000,"posture_at_import":20000,"provider_message_ids":20000,"verdict_reason":2000,"verdict_reasons":2000,"verdict_status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_import" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"delivered_by":20000,"posture_at_import":20000,"provider_message_ids":20000,"verdict_reason":2000,"verdict_reasons":2000,"verdict_status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"delivered_by":20000,"posture_at_import":20000,"provider_message_ids":20000,"verdict_reason":2000,"verdict_reasons":2000,"verdict_status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_owner_identity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"created_by":20000,"kind":20000,"source":20000,"value":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"created_by":20000,"kind":20000,"source":20000,"value":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_owner_identity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"created_by":20000,"kind":20000,"source":20000,"value":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"created_by":20000,"kind":20000,"source":20000,"value":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_owner_identity_withdrawn" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"created_by":20000,"kind":20000,"source":20000,"value":20000,"withdrawn_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"created_by":20000,"kind":20000,"source":20000,"value":20000,"withdrawn_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_owner_identity_withdrawn" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"created_by":20000,"kind":20000,"source":20000,"value":20000,"withdrawn_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"created_by":20000,"kind":20000,"source":20000,"value":20000,"withdrawn_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_party_source" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"containers":20000,"policy_addresses":20000,"provider":20000,"recipients":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"containers":20000,"policy_addresses":20000,"provider":20000,"recipients":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_party_source" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"containers":20000,"policy_addresses":20000,"provider":20000,"recipients":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"containers":20000,"policy_addresses":20000,"provider":20000,"recipients":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_party_source_replay" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_party_source_replay" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_pending_counterparty" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"disposition_reason":2000,"domain":20000,"email":20000,"kind":20000,"served_model":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"disposition_reason":2000,"domain":20000,"email":20000,"kind":20000,"served_model":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_pending_counterparty" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"disposition_reason":2000,"domain":20000,"email":20000,"kind":20000,"served_model":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"disposition_reason":2000,"domain":20000,"email":20000,"kind":20000,"served_model":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_sender_override" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"address":20000,"captured_by":20000,"decision":20000,"overruled_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address":20000,"captured_by":20000,"decision":20000,"overruled_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_sender_override" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"address":20000,"captured_by":20000,"decision":20000,"overruled_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address":20000,"captured_by":20000,"decision":20000,"overruled_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_sweep_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"error_class":20000,"outcome":20000,"sweep":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"error_class":20000,"outcome":20000,"sweep":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_sweep_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"error_class":20000,"outcome":20000,"sweep":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"error_class":20000,"outcome":20000,"sweep":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_sync_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"last_error_class":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"last_error_class":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_sync_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"last_error_class":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"last_error_class":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_test_mailbox_sent" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"subject":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"subject":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_test_mailbox_sent" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"subject":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"subject":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_thread_verdict" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"disposition_reason":2000,"kind":20000,"seen_addresses":20000,"status":20000,"thread_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"disposition_reason":2000,"kind":20000,"seen_addresses":20000,"status":20000,"thread_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_thread_verdict" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"disposition_reason":2000,"kind":20000,"seen_addresses":20000,"status":20000,"thread_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"disposition_reason":2000,"kind":20000,"seen_addresses":20000,"status":20000,"thread_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."channel_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"channel_id":20000,"channel_label":120,"credential_ref":20000,"provider":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"channel_id":20000,"channel_label":120,"credential_ref":20000,"provider":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."channel_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"channel_id":20000,"channel_label":120,"credential_ref":20000,"provider":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"channel_id":20000,"channel_label":120,"credential_ref":20000,"provider":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."channel_provider" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"credential_model":20000,"label":120,"transport":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"credential_model":20000,"label":120,"transport":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."channel_provider" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"credential_model":20000,"label":120,"transport":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"credential_model":20000,"label":120,"transport":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."close_date_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."close_date_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."close_date_run_member" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."close_date_run_member" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."commission_entry" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"attribution_at_accrual":20000,"captured_by":20000,"margin_tier_at_accrual":20000,"status":20000,"void_reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"attribution_at_accrual":20000,"captured_by":20000,"margin_tier_at_accrual":20000,"status":20000,"void_reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."commission_entry" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"attribution_at_accrual":20000,"captured_by":20000,"margin_tier_at_accrual":20000,"status":20000,"void_reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"attribution_at_accrual":20000,"captured_by":20000,"margin_tier_at_accrual":20000,"status":20000,"void_reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."comms_outbound" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"bounce_kind":20000,"bounce_reason":2000,"bounce_recipient":20000,"channel_user_id":20000,"consent_purpose":20000,"execution_authority":20000,"from_address":20000,"in_reply_to":20000,"inline_logo_key":20000,"list_unsubscribe":20000,"message_id":20000,"payload_ref":20000,"provider":20000,"provider_message_id":20000,"reason":2000,"redacted_fields":20000,"sender_kind":20000,"status":20000,"subject":20000,"template_key":20000,"thread_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"bounce_kind":20000,"bounce_reason":2000,"bounce_recipient":20000,"channel_user_id":20000,"consent_purpose":20000,"execution_authority":20000,"from_address":20000,"in_reply_to":20000,"inline_logo_key":20000,"list_unsubscribe":20000,"message_id":20000,"payload_ref":20000,"provider":20000,"provider_message_id":20000,"reason":2000,"redacted_fields":20000,"sender_kind":20000,"status":20000,"subject":20000,"template_key":20000,"thread_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."comms_outbound" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"bounce_kind":20000,"bounce_reason":2000,"bounce_recipient":20000,"channel_user_id":20000,"consent_purpose":20000,"execution_authority":20000,"from_address":20000,"in_reply_to":20000,"inline_logo_key":20000,"list_unsubscribe":20000,"message_id":20000,"payload_ref":20000,"provider":20000,"provider_message_id":20000,"reason":2000,"redacted_fields":20000,"sender_kind":20000,"status":20000,"subject":20000,"template_key":20000,"thread_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"bounce_kind":20000,"bounce_reason":2000,"bounce_recipient":20000,"channel_user_id":20000,"consent_purpose":20000,"execution_authority":20000,"from_address":20000,"in_reply_to":20000,"inline_logo_key":20000,"list_unsubscribe":20000,"message_id":20000,"payload_ref":20000,"provider":20000,"provider_message_id":20000,"reason":2000,"redacted_fields":20000,"sender_kind":20000,"status":20000,"subject":20000,"template_key":20000,"thread_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."communication_basis" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"kind":20000,"note":2000,"thread_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"kind":20000,"note":2000,"thread_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."communication_basis" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"kind":20000,"note":2000,"thread_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"kind":20000,"note":2000,"thread_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."communication_decision" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"actor":20000,"basis":20000,"data_protection_country":20000,"execution_authority":20000,"legacy_verdict":20000,"mode":20000,"phase":20000,"purpose_key":20000,"reason_code":2000,"recipient_address":20000,"requested_category":20000,"resolved_category":20000,"ruleset_codes":20000,"subject_kind":20000,"suppression":20000,"verdict":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"actor":20000,"basis":20000,"data_protection_country":20000,"execution_authority":20000,"legacy_verdict":20000,"mode":20000,"phase":20000,"purpose_key":20000,"reason_code":2000,"recipient_address":20000,"requested_category":20000,"resolved_category":20000,"ruleset_codes":20000,"subject_kind":20000,"suppression":20000,"verdict":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."communication_decision" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"actor":20000,"basis":20000,"data_protection_country":20000,"execution_authority":20000,"legacy_verdict":20000,"mode":20000,"phase":20000,"purpose_key":20000,"reason_code":2000,"recipient_address":20000,"requested_category":20000,"resolved_category":20000,"ruleset_codes":20000,"subject_kind":20000,"suppression":20000,"verdict":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"actor":20000,"basis":20000,"data_protection_country":20000,"execution_authority":20000,"legacy_verdict":20000,"mode":20000,"phase":20000,"purpose_key":20000,"reason_code":2000,"recipient_address":20000,"requested_category":20000,"resolved_category":20000,"ruleset_codes":20000,"subject_kind":20000,"suppression":20000,"verdict":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."communication_instruction" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"directed_by_agent":20000,"explanation":20000,"reason_code":2000,"revoked_reason":2000,"status":20000,"warning_version":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"directed_by_agent":20000,"explanation":20000,"reason_code":2000,"revoked_reason":2000,"status":20000,"warning_version":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."communication_instruction" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"directed_by_agent":20000,"explanation":20000,"reason_code":2000,"revoked_reason":2000,"status":20000,"warning_version":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"directed_by_agent":20000,"explanation":20000,"reason_code":2000,"revoked_reason":2000,"status":20000,"warning_version":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."communication_override" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"category":20000,"decided_by_level":20000,"reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"category":20000,"decided_by_level":20000,"reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."communication_override" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"category":20000,"decided_by_level":20000,"reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"category":20000,"decided_by_level":20000,"reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."communication_review" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"kind":20000,"reason_code":2000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000,"reason_code":2000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."communication_review" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"kind":20000,"reason_code":2000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000,"reason_code":2000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."communication_suppression" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"address":20000,"address_hash":20000,"address_key_id":20000,"captured_by":20000,"decided_by_level":20000,"kind":20000,"recorded_for_addresses":20000,"recorded_through":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address":20000,"address_hash":20000,"address_key_id":20000,"captured_by":20000,"decided_by_level":20000,"kind":20000,"recorded_for_addresses":20000,"recorded_through":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."communication_suppression" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"address":20000,"address_hash":20000,"address_key_id":20000,"captured_by":20000,"decided_by_level":20000,"kind":20000,"recorded_for_addresses":20000,"recorded_through":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address":20000,"address_hash":20000,"address_key_id":20000,"captured_by":20000,"decided_by_level":20000,"kind":20000,"recorded_for_addresses":20000,"recorded_through":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"address_city":20000,"address_country":20000,"address_line1":500,"address_line2":500,"address_postal_code":20000,"address_region":20000,"captured_by":20000,"description":20000,"email":20000,"geocode_input_hash":20000,"geocode_provider":20000,"geocode_status":20000,"industry":20000,"legal_name":500,"lifecycle":20000,"linkedin_url":20000,"logo_icon_object_key":20000,"logo_icon_origin":20000,"logo_object_key":20000,"logo_origin":20000,"name_source":20000,"phone":20000,"size_band":20000,"source":20000,"source_system":20000,"visibility":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address_city":20000,"address_country":20000,"address_line1":500,"address_line2":500,"address_postal_code":20000,"address_region":20000,"captured_by":20000,"description":20000,"email":20000,"geocode_input_hash":20000,"geocode_provider":20000,"geocode_status":20000,"industry":20000,"legal_name":500,"lifecycle":20000,"linkedin_url":20000,"logo_icon_object_key":20000,"logo_icon_origin":20000,"logo_object_key":20000,"logo_origin":20000,"name_source":20000,"phone":20000,"size_band":20000,"source":20000,"source_system":20000,"visibility":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"address_city":20000,"address_country":20000,"address_line1":500,"address_line2":500,"address_postal_code":20000,"address_region":20000,"captured_by":20000,"description":20000,"email":20000,"geocode_input_hash":20000,"geocode_provider":20000,"geocode_status":20000,"industry":20000,"legal_name":500,"lifecycle":20000,"linkedin_url":20000,"logo_icon_object_key":20000,"logo_icon_origin":20000,"logo_object_key":20000,"logo_origin":20000,"name_source":20000,"phone":20000,"size_band":20000,"source":20000,"source_system":20000,"visibility":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address_city":20000,"address_country":20000,"address_line1":500,"address_line2":500,"address_postal_code":20000,"address_region":20000,"captured_by":20000,"description":20000,"email":20000,"geocode_input_hash":20000,"geocode_provider":20000,"geocode_status":20000,"industry":20000,"legal_name":500,"lifecycle":20000,"linkedin_url":20000,"logo_icon_object_key":20000,"logo_icon_origin":20000,"logo_object_key":20000,"logo_origin":20000,"name_source":20000,"phone":20000,"size_band":20000,"source":20000,"source_system":20000,"visibility":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_address_country" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"countries":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"countries":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_address_country" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"countries":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"countries":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_brief" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_brief" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_domain" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"domain":20000,"domain_folded":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"domain":20000,"domain_folded":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_domain" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"domain":20000,"domain_folded":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"domain":20000,"domain_folded":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_domain_disposition" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"admission":20000,"admission_reason":2000,"admission_reason_code":2000,"admission_sender_kind":20000,"admission_source":20000,"domain":20000,"evidence":20000,"pending_reason":2000,"source":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"admission":20000,"admission_reason":2000,"admission_reason_code":2000,"admission_sender_kind":20000,"admission_source":20000,"domain":20000,"evidence":20000,"pending_reason":2000,"source":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_domain_disposition" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"admission":20000,"admission_reason":2000,"admission_reason_code":2000,"admission_sender_kind":20000,"admission_source":20000,"domain":20000,"evidence":20000,"pending_reason":2000,"source":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"admission":20000,"admission_reason":2000,"admission_reason_code":2000,"admission_sender_kind":20000,"admission_source":20000,"domain":20000,"evidence":20000,"pending_reason":2000,"source":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_domain_question_reader" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"domain":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"domain":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_domain_question_reader" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"domain":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"domain":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_dossier" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_dossier" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_fact" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"category":20000,"evidence_snippet":20000,"field":20000,"source":20000,"source_url":20000,"value":20000,"value_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"category":20000,"evidence_snippet":20000,"field":20000,"source":20000,"source_url":20000,"value":20000,"value_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_fact" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"category":20000,"evidence_snippet":20000,"field":20000,"source":20000,"source_url":20000,"value":20000,"value_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"category":20000,"evidence_snippet":20000,"field":20000,"source":20000,"source_url":20000,"value":20000,"value_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_geocode_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"input_hash":20000,"last_outcome":20000,"provider":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"input_hash":20000,"last_outcome":20000,"provider":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_geocode_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"input_hash":20000,"last_outcome":20000,"provider":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"input_hash":20000,"last_outcome":20000,"provider":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_growth_fit" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_growth_fit" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_offered_value" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"field_key":20000,"offered_value":20000,"source_system":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"field_key":20000,"offered_value":20000,"source_system":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_offered_value" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"field_key":20000,"offered_value":20000,"source_system":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"field_key":20000,"offered_value":20000,"source_system":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_profile_field" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"evidence_snippet":20000,"field":20000,"source":20000,"source_url":20000,"value":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"evidence_snippet":20000,"field":20000,"source":20000,"source_url":20000,"value":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_profile_field" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"evidence_snippet":20000,"field":20000,"source":20000,"source_url":20000,"value":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"evidence_snippet":20000,"field":20000,"source":20000,"source_url":20000,"value":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_relationship_type" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"relationship_type":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"relationship_type":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_relationship_type" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"relationship_type":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"relationship_type":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_scan" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"degrade_reason":2000,"fingerprint":20000,"generated_by":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"degrade_reason":2000,"fingerprint":20000,"generated_by":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_scan" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"degrade_reason":2000,"fingerprint":20000,"generated_by":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"degrade_reason":2000,"fingerprint":20000,"generated_by":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_technical_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"lane":20000,"last_outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"lane":20000,"last_outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_technical_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"lane":20000,"last_outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"lane":20000,"last_outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_vat_check" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"consultation_number":20000,"registered_address":20000,"registered_name":120,"status":20000,"vat_number":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"consultation_number":20000,"registered_address":20000,"registered_name":120,"status":20000,"vat_number":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_vat_check" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"consultation_number":20000,"registered_address":20000,"registered_name":120,"status":20000,"vat_number":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"consultation_number":20000,"registered_address":20000,"registered_name":120,"status":20000,"vat_number":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."confirm_token" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"delivered_to":20000,"kind":20000,"question_key":20000,"question_locale":20000,"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"delivered_to":20000,"kind":20000,"question_key":20000,"question_locale":20000,"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."confirm_token" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"delivered_to":20000,"kind":20000,"question_key":20000,"question_locale":20000,"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"delivered_to":20000,"kind":20000,"question_key":20000,"question_locale":20000,"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."consent_doi_token" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."consent_doi_token" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."consent_event" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"confirm_ip":20000,"confirm_user_agent":20000,"issuance_trigger":20000,"lawful_basis":20000,"new_state":20000,"policy_text":20000,"policy_version":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"confirm_ip":20000,"confirm_user_agent":20000,"issuance_trigger":20000,"lawful_basis":20000,"new_state":20000,"policy_text":20000,"policy_version":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."consent_event" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"confirm_ip":20000,"confirm_user_agent":20000,"issuance_trigger":20000,"lawful_basis":20000,"new_state":20000,"policy_text":20000,"policy_version":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"confirm_ip":20000,"confirm_user_agent":20000,"issuance_trigger":20000,"lawful_basis":20000,"new_state":20000,"policy_text":20000,"policy_version":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."consent_purpose" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"class":20000,"key":20000,"label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"class":20000,"key":20000,"label":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."consent_purpose" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"class":20000,"key":20000,"label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"class":20000,"key":20000,"label":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."consent_qualifying_event" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"kind":20000,"note":2000,"recorded_for_addresses":20000,"retracted_by":20000,"source":20000,"source_entity_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"kind":20000,"note":2000,"recorded_for_addresses":20000,"retracted_by":20000,"source":20000,"source_entity_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."consent_qualifying_event" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"kind":20000,"note":2000,"recorded_for_addresses":20000,"retracted_by":20000,"source":20000,"source_entity_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"kind":20000,"note":2000,"recorded_for_addresses":20000,"retracted_by":20000,"source":20000,"source_entity_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."consent_text_version" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"channel":20000,"content_hash":20000,"jurisdiction":20000,"key":20000,"locale":20000,"subject_line":20000,"version":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"channel":20000,"content_hash":20000,"jurisdiction":20000,"key":20000,"locale":20000,"subject_line":20000,"version":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."consent_text_version" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"channel":20000,"content_hash":20000,"jurisdiction":20000,"key":20000,"locale":20000,"subject_line":20000,"version":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"channel":20000,"content_hash":20000,"jurisdiction":20000,"key":20000,"locale":20000,"subject_line":20000,"version":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"address_city":20000,"address_country":20000,"address_line1":500,"address_line2":500,"address_postal_code":20000,"address_region":20000,"anonymized_address_hashes":20000,"anonymized_address_key_id":20000,"captured_by":20000,"narrowing_reason":2000,"photo_object_key":20000,"photo_origin":20000,"relationship_mark":20000,"relationship_mark_set_by":20000,"source":20000,"source_system":20000,"title":120,"visibility":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address_city":20000,"address_country":20000,"address_line1":500,"address_line2":500,"address_postal_code":20000,"address_region":20000,"anonymized_address_hashes":20000,"anonymized_address_key_id":20000,"captured_by":20000,"narrowing_reason":2000,"photo_object_key":20000,"photo_origin":20000,"relationship_mark":20000,"relationship_mark_set_by":20000,"source":20000,"source_system":20000,"title":120,"visibility":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"address_city":20000,"address_country":20000,"address_line1":500,"address_line2":500,"address_postal_code":20000,"address_region":20000,"anonymized_address_hashes":20000,"anonymized_address_key_id":20000,"captured_by":20000,"narrowing_reason":2000,"photo_object_key":20000,"photo_origin":20000,"relationship_mark":20000,"relationship_mark_set_by":20000,"source":20000,"source_system":20000,"title":120,"visibility":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address_city":20000,"address_country":20000,"address_line1":500,"address_line2":500,"address_postal_code":20000,"address_region":20000,"anonymized_address_hashes":20000,"anonymized_address_key_id":20000,"captured_by":20000,"narrowing_reason":2000,"photo_object_key":20000,"photo_origin":20000,"relationship_mark":20000,"relationship_mark_set_by":20000,"source":20000,"source_system":20000,"title":120,"visibility":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_acquisition_evidence" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"kind":20000,"note":2000,"purpose_claimed":20000,"recorded_for_addresses":20000,"request_retracted_by":20000,"source_entity_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"kind":20000,"note":2000,"purpose_claimed":20000,"recorded_for_addresses":20000,"request_retracted_by":20000,"source_entity_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_acquisition_evidence" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"kind":20000,"note":2000,"purpose_claimed":20000,"recorded_for_addresses":20000,"request_retracted_by":20000,"source_entity_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"kind":20000,"note":2000,"purpose_claimed":20000,"recorded_for_addresses":20000,"request_retracted_by":20000,"source_entity_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_address_country" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"countries":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"countries":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_address_country" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"countries":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"countries":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_brief" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_brief" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_channel_identity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"channel_user_id":20000,"membership_bot_id":20000,"provider":20000,"source":20000,"username":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"channel_user_id":20000,"membership_bot_id":20000,"provider":20000,"source":20000,"username":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_channel_identity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"channel_user_id":20000,"membership_bot_id":20000,"provider":20000,"source":20000,"username":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"channel_user_id":20000,"membership_bot_id":20000,"provider":20000,"source":20000,"username":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_confirm_submission" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"field":20000,"kind":20000,"note":2000,"proposed_value":20000,"resolution":20000,"resolved_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"field":20000,"kind":20000,"note":2000,"proposed_value":20000,"resolution":20000,"resolved_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_confirm_submission" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"field":20000,"kind":20000,"note":2000,"proposed_value":20000,"resolution":20000,"resolved_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"field":20000,"kind":20000,"note":2000,"proposed_value":20000,"resolution":20000,"resolved_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_consent" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"lawful_basis":20000,"policy_version":20000,"recorded_for_addresses":20000,"source":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"lawful_basis":20000,"policy_version":20000,"recorded_for_addresses":20000,"source":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_consent" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"lawful_basis":20000,"policy_version":20000,"recorded_for_addresses":20000,"source":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"lawful_basis":20000,"policy_version":20000,"recorded_for_addresses":20000,"source":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_email" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"email":20000,"email_domain_folded":20000,"email_type":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"email":20000,"email_domain_folded":20000,"email_type":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_email" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"email":20000,"email_domain_folded":20000,"email_type":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"email":20000,"email_domain_folded":20000,"email_type":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_moment_dismissal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"claim_key":20000,"evidence_fingerprint":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"claim_key":20000,"evidence_fingerprint":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_moment_dismissal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"claim_key":20000,"evidence_fingerprint":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"claim_key":20000,"evidence_fingerprint":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_phone" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"phone":20000,"phone_type":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"phone":20000,"phone_type":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_phone" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"phone":20000,"phone_type":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"phone":20000,"phone_type":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_profile_field" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"evidence_snippet":20000,"field":20000,"source":20000,"source_ref":20000,"superseded_captured_by":20000,"superseded_value":20000,"value":20000,"value_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"evidence_snippet":20000,"field":20000,"source":20000,"source_ref":20000,"superseded_captured_by":20000,"superseded_value":20000,"value":20000,"value_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_profile_field" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"evidence_snippet":20000,"field":20000,"source":20000,"source_ref":20000,"superseded_captured_by":20000,"superseded_value":20000,"value":20000,"value_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"evidence_snippet":20000,"field":20000,"source":20000,"source_ref":20000,"superseded_captured_by":20000,"superseded_value":20000,"value":20000,"value_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_provider_claim" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"claim_key":20000,"employment_processing_error":20000,"provider":20000,"source":20000,"validation_status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"claim_key":20000,"employment_processing_error":20000,"provider":20000,"source":20000,"validation_status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_provider_claim" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"claim_key":20000,"employment_processing_error":20000,"provider":20000,"source":20000,"validation_status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"claim_key":20000,"employment_processing_error":20000,"provider":20000,"source":20000,"validation_status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_social" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"handle":20000,"platform":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"handle":20000,"platform":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_social" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"handle":20000,"platform":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"handle":20000,"platform":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contract" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"contract_number":20000,"source":20000,"status":20000,"title":120,"value_basis":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"contract_number":20000,"source":20000,"status":20000,"title":120,"value_basis":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contract" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"contract_number":20000,"source":20000,"status":20000,"title":120,"value_basis":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"contract_number":20000,"source":20000,"status":20000,"title":120,"value_basis":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."conversation_claim" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"body":20000,"captured_by":20000,"evidence_fingerprint":20000,"kind":20000,"source":20000,"source_quote":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"body":20000,"captured_by":20000,"evidence_fingerprint":20000,"kind":20000,"source":20000,"source_quote":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."conversation_claim" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"body":20000,"captured_by":20000,"evidence_fingerprint":20000,"kind":20000,"source":20000,"source_quote":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"body":20000,"captured_by":20000,"evidence_fingerprint":20000,"kind":20000,"source":20000,"source_quote":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."custom_field" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"activity_kinds":20000,"column_name":120,"label":120,"object":20000,"slug":20000,"status":20000,"type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"activity_kinds":20000,"column_name":120,"label":120,"object":20000,"slug":20000,"status":20000,"type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."custom_field" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"activity_kinds":20000,"column_name":120,"label":120,"object":20000,"slug":20000,"status":20000,"type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"activity_kinds":20000,"column_name":120,"label":120,"object":20000,"slug":20000,"status":20000,"type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."data_subject_request" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"channel":20000,"due_rule":20000,"kind":20000,"receipt_reference":20000,"resolution":20000,"status":20000,"subject_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"channel":20000,"due_rule":20000,"kind":20000,"receipt_reference":20000,"resolution":20000,"status":20000,"subject_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."data_subject_request" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"channel":20000,"due_rule":20000,"kind":20000,"receipt_reference":20000,"resolution":20000,"status":20000,"subject_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"channel":20000,"due_rule":20000,"kind":20000,"receipt_reference":20000,"resolution":20000,"status":20000,"subject_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."data_task_review" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"operator":20000,"task_id":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"operator":20000,"task_id":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."data_task_review" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"operator":20000,"task_id":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"operator":20000,"task_id":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."data_task_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"first_release":20000,"last_error_class":20000,"status":20000,"step":20000,"task_id":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"first_release":20000,"last_error_class":20000,"status":20000,"step":20000,"task_id":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."data_task_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"first_release":20000,"last_error_class":20000,"status":20000,"step":20000,"task_id":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"first_release":20000,"last_error_class":20000,"status":20000,"step":20000,"task_id":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."data_task_run_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"step":20000,"task_id":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"step":20000,"task_id":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."data_task_run_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"step":20000,"task_id":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"step":20000,"task_id":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"acquisition_source":20000,"captured_by":20000,"commercial_mode":20000,"commercial_motion":20000,"description":20000,"lost_reason":2000,"lost_reason_basis":2000,"lost_reason_code":2000,"name":120,"partner_attribution":20000,"priority":20000,"source":20000,"source_system":20000,"status":20000,"win_probability_set_by":20000,"won_without_contract_detail":20000,"won_without_contract_reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"acquisition_source":20000,"captured_by":20000,"commercial_mode":20000,"commercial_motion":20000,"description":20000,"lost_reason":2000,"lost_reason_basis":2000,"lost_reason_code":2000,"name":120,"partner_attribution":20000,"priority":20000,"source":20000,"source_system":20000,"status":20000,"win_probability_set_by":20000,"won_without_contract_detail":20000,"won_without_contract_reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"acquisition_source":20000,"captured_by":20000,"commercial_mode":20000,"commercial_motion":20000,"description":20000,"lost_reason":2000,"lost_reason_basis":2000,"lost_reason_code":2000,"name":120,"partner_attribution":20000,"priority":20000,"source":20000,"source_system":20000,"status":20000,"win_probability_set_by":20000,"won_without_contract_detail":20000,"won_without_contract_reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"acquisition_source":20000,"captured_by":20000,"commercial_mode":20000,"commercial_motion":20000,"description":20000,"lost_reason":2000,"lost_reason_basis":2000,"lost_reason_code":2000,"name":120,"partner_attribution":20000,"priority":20000,"source":20000,"source_system":20000,"status":20000,"win_probability_set_by":20000,"won_without_contract_detail":20000,"won_without_contract_reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_acquisition_source" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"key":20000,"label":120,"shipped_label":120,"shipped_label_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key":20000,"label":120,"shipped_label":120,"shipped_label_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_acquisition_source" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"key":20000,"label":120,"shipped_label":120,"shipped_label_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key":20000,"label":120,"shipped_label":120,"shipped_label_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_correction" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"asking":20000,"captured_by":20000,"correction":20000,"fields":20000,"remaining_open_stages":20000,"reversed_by":20000,"standing_close_date":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"asking":20000,"captured_by":20000,"correction":20000,"fields":20000,"remaining_open_stages":20000,"reversed_by":20000,"standing_close_date":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_correction" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"asking":20000,"captured_by":20000,"correction":20000,"fields":20000,"remaining_open_stages":20000,"reversed_by":20000,"standing_close_date":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"asking":20000,"captured_by":20000,"correction":20000,"fields":20000,"remaining_open_stages":20000,"reversed_by":20000,"standing_close_date":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_document_hide" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"hidden_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"hidden_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_document_hide" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"hidden_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"hidden_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_line_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"billing_model":20000,"captured_by":20000,"catalog_name":500,"component_kind":20000,"description":20000,"sku":500,"tax_category":20000,"unit":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"billing_model":20000,"captured_by":20000,"catalog_name":500,"component_kind":20000,"description":20000,"sku":500,"tax_category":20000,"unit":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_line_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"billing_model":20000,"captured_by":20000,"catalog_name":500,"component_kind":20000,"description":20000,"sku":500,"tax_category":20000,"unit":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"billing_model":20000,"captured_by":20000,"catalog_name":500,"component_kind":20000,"description":20000,"sku":500,"tax_category":20000,"unit":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_loss_reason" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"key":20000,"label":120,"shipped_label":120,"shipped_label_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key":20000,"label":120,"shipped_label":120,"shipped_label_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_loss_reason" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"key":20000,"label":120,"shipped_label":120,"shipped_label_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key":20000,"label":120,"shipped_label":120,"shipped_label_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_loss_reason_mapping" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"decided_by":20000,"note_key":20000,"reason_key":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"decided_by":20000,"note_key":20000,"reason_key":2000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_loss_reason_mapping" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"decided_by":20000,"note_key":20000,"reason_key":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"decided_by":20000,"note_key":20000,"reason_key":2000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_move_dismissal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"fingerprint":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_move_dismissal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"fingerprint":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_product" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"component_kind":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"component_kind":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_product" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"component_kind":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"component_kind":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_risk_day" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_risk_day" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_room" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"source":20000,"state":20000,"title":120,"welcome_message":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"source":20000,"state":20000,"title":120,"welcome_message":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_room" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"source":20000,"state":20000,"title":120,"welcome_message":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"source":20000,"state":20000,"title":120,"welcome_message":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_room_comment" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"body":20000,"captured_by":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"body":20000,"captured_by":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_room_comment" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"body":20000,"captured_by":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"body":20000,"captured_by":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_room_document" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"group_key":20000,"source":20000,"title":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"group_key":20000,"source":20000,"title":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_room_document" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"group_key":20000,"source":20000,"title":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"group_key":20000,"source":20000,"title":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_room_engagement" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_room_engagement" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_room_invitation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"failure_reason":2000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"failure_reason":2000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_room_invitation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"failure_reason":2000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"failure_reason":2000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_room_participant" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"capability":20000,"captured_by":20000,"email":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"capability":20000,"captured_by":20000,"email":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_room_participant" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"capability":20000,"captured_by":20000,"email":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"capability":20000,"captured_by":20000,"email":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_room_session" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_room_session" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_room_thread" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"source":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"source":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_room_thread" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"source":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"source":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_stage_evidence" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"author_side":20000,"commitment":20000,"extracted_by":20000,"snippet":500,"source_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"author_side":20000,"commitment":20000,"extracted_by":20000,"snippet":500,"source_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_stage_evidence" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"author_side":20000,"commitment":20000,"extracted_by":20000,"snippet":500,"source_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"author_side":20000,"commitment":20000,"extracted_by":20000,"snippet":500,"source_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_stage_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"changed_by":20000,"lost_reason_code_at_change":2000,"provenance":20000,"semantic_at_change":20000,"valuation_provenance":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"changed_by":20000,"lost_reason_code_at_change":2000,"provenance":20000,"semantic_at_change":20000,"valuation_provenance":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_stage_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"changed_by":20000,"lost_reason_code_at_change":2000,"provenance":20000,"semantic_at_change":20000,"valuation_provenance":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"changed_by":20000,"lost_reason_code_at_change":2000,"provenance":20000,"semantic_at_change":20000,"valuation_provenance":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_status_card" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_status_card" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_suggestion" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"fingerprint":20000,"kind":20000,"name_hint":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"fingerprint":20000,"kind":20000,"name_hint":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_suggestion" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"fingerprint":20000,"kind":20000,"name_hint":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"fingerprint":20000,"kind":20000,"name_hint":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_suggestion_evidence" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_suggestion_evidence" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."dedupe_candidate" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"disposition":20000,"entity_type":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"disposition":20000,"entity_type":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."dedupe_candidate" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"disposition":20000,"entity_type":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"disposition":20000,"entity_type":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."dedupe_scan_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"stream":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"stream":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."dedupe_scan_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"stream":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"stream":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."email_signature" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"phone":20000,"title":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"phone":20000,"title":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."email_signature" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"phone":20000,"title":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"phone":20000,"title":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."embed_store_binding" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"populated_identity":20000,"reembedding_identity":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"populated_identity":20000,"reembedding_identity":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."embed_store_binding" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"populated_identity":20000,"reembedding_identity":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"populated_identity":20000,"reembedding_identity":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."embedding" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"chunk_hash":20000,"entity_type":20000,"model":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"chunk_hash":20000,"entity_type":20000,"model":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."embedding" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"chunk_hash":20000,"entity_type":20000,"model":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"chunk_hash":20000,"entity_type":20000,"model":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."erasure_suppression" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"key_id":20000,"kind":20000,"value_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key_id":20000,"kind":20000,"value_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."erasure_suppression" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"key_id":20000,"kind":20000,"value_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key_id":20000,"kind":20000,"value_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."existing_customer_notice" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"collection_channel":20000,"customer_group":20000,"evidence_reference":20000,"notice_text":20000,"withdrawn_by":20000,"withdrawn_reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"collection_channel":20000,"customer_group":20000,"evidence_reference":20000,"notice_text":20000,"withdrawn_by":20000,"withdrawn_reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."existing_customer_notice" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"collection_channel":20000,"customer_group":20000,"evidence_reference":20000,"notice_text":20000,"withdrawn_by":20000,"withdrawn_reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"collection_channel":20000,"customer_group":20000,"evidence_reference":20000,"notice_text":20000,"withdrawn_by":20000,"withdrawn_reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."extension_ingest_position" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"position":20000,"source_system":20000,"unit":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"position":20000,"source_system":20000,"unit":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."extension_ingest_position" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"position":20000,"source_system":20000,"unit":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"position":20000,"source_system":20000,"unit":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."extension_ingest_refusal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"refusal":20000,"unit":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"refusal":20000,"unit":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."extension_ingest_refusal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"refusal":20000,"unit":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"refusal":20000,"unit":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."extension_secret" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"extension_name":120,"key":20000,"vault_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"extension_name":120,"key":20000,"vault_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."extension_secret" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"extension_name":120,"key":20000,"vault_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"extension_name":120,"key":20000,"vault_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."federated_identity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"email_at_link":20000,"provider":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"email_at_link":20000,"provider":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."federated_identity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"email_at_link":20000,"provider":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"email_at_link":20000,"provider":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."field_mask" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"condition":20000,"field":20000,"object":20000,"role_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"condition":20000,"field":20000,"object":20000,"role_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."field_mask" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"condition":20000,"field":20000,"object":20000,"role_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"condition":20000,"field":20000,"object":20000,"role_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."field_provenance" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"evidence_ref":20000,"field_name":120,"object_type":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"evidence_ref":20000,"field_name":120,"object_type":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."field_provenance" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"evidence_ref":20000,"field_name":120,"object_type":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"evidence_ref":20000,"field_name":120,"object_type":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."finance_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"credential_ref":20000,"last_error_code":20000,"provider":20000,"source":20000,"status":20000,"sync_cursor":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"credential_ref":20000,"last_error_code":20000,"provider":20000,"source":20000,"status":20000,"sync_cursor":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."finance_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"credential_ref":20000,"last_error_code":20000,"provider":20000,"source":20000,"status":20000,"sync_cursor":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"credential_ref":20000,"last_error_code":20000,"provider":20000,"source":20000,"status":20000,"sync_cursor":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."finance_customer_link" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"external_customer_id":20000,"source":20000,"sync_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"external_customer_id":20000,"source":20000,"sync_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."finance_customer_link" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"external_customer_id":20000,"source":20000,"sync_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"external_customer_id":20000,"source":20000,"sync_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."finance_external_customer" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"external_customer_id":20000,"source":20000,"sync_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"external_customer_id":20000,"source":20000,"sync_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."finance_external_customer" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"external_customer_id":20000,"source":20000,"sync_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"external_customer_id":20000,"source":20000,"sync_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."finance_invoice" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"external_id":20000,"number":20000,"raw_status":20000,"source":20000,"status":20000,"sync_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"external_id":20000,"number":20000,"raw_status":20000,"source":20000,"status":20000,"sync_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."finance_invoice" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"external_id":20000,"number":20000,"raw_status":20000,"source":20000,"status":20000,"sync_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"external_id":20000,"number":20000,"raw_status":20000,"source":20000,"status":20000,"sync_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."finance_payment" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"external_id":20000,"source":20000,"sync_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"external_id":20000,"source":20000,"sync_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."finance_payment" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"external_id":20000,"source":20000,"sync_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"external_id":20000,"source":20000,"sync_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."forecast_call" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"note":2000,"scope_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"note":2000,"scope_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."forecast_call" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"note":2000,"scope_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"note":2000,"scope_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."forecast_capture_status" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"context_key":20000,"failure":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"context_key":20000,"failure":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."forecast_capture_status" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"context_key":20000,"failure":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"context_key":20000,"failure":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."forecast_contribution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"category":20000,"exclusion_reason":2000,"probability_source":20000,"qualification":20000,"qualified_policy_version":20000,"risk_signals":20000,"viability_disposition":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"category":20000,"exclusion_reason":2000,"probability_source":20000,"qualification":20000,"qualified_policy_version":20000,"risk_signals":20000,"viability_disposition":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."forecast_contribution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"category":20000,"exclusion_reason":2000,"probability_source":20000,"qualification":20000,"qualified_policy_version":20000,"risk_signals":20000,"viability_disposition":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"category":20000,"exclusion_reason":2000,"probability_source":20000,"qualification":20000,"qualified_policy_version":20000,"risk_signals":20000,"viability_disposition":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."forecast_snapshot" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"definition_version":20000,"forward_measure":20000,"input_fingerprint":20000,"landing_measure":20000,"population_fingerprint":20000,"qualified_policy_version":20000,"range_quantiles":20000,"range_version":20000,"risk_signals_version":20000,"scope_kind":20000,"trigger":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"definition_version":20000,"forward_measure":20000,"input_fingerprint":20000,"landing_measure":20000,"population_fingerprint":20000,"qualified_policy_version":20000,"range_quantiles":20000,"range_version":20000,"risk_signals_version":20000,"scope_kind":20000,"trigger":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."forecast_snapshot" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"definition_version":20000,"forward_measure":20000,"input_fingerprint":20000,"landing_measure":20000,"population_fingerprint":20000,"qualified_policy_version":20000,"range_quantiles":20000,"range_version":20000,"risk_signals_version":20000,"scope_kind":20000,"trigger":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"definition_version":20000,"forward_measure":20000,"input_fingerprint":20000,"landing_measure":20000,"population_fingerprint":20000,"qualified_policy_version":20000,"range_quantiles":20000,"range_version":20000,"risk_signals_version":20000,"scope_kind":20000,"trigger":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."geocode_cache" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"provider":20000,"query":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"provider":20000,"query":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."geocode_cache" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"provider":20000,"query":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"provider":20000,"query":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."geocode_country_cache" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"country_codes":20000,"provider":20000,"query":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"country_codes":20000,"provider":20000,"query":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."geocode_country_cache" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"country_codes":20000,"provider":20000,"query":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"country_codes":20000,"provider":20000,"query":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."home_read_cache" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"read":20000,"request_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"read":20000,"request_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."home_read_cache" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"read":20000,"request_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"read":20000,"request_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."home_read_fence_pending" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"fence":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fence":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."home_read_fence_pending" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"fence":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fence":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."idempotency_key" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"endpoint":20000,"key":20000,"principal_id":20000,"request_digest":20000,"response_content_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"endpoint":20000,"key":20000,"principal_id":20000,"request_digest":20000,"response_content_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."idempotency_key" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"endpoint":20000,"key":20000,"principal_id":20000,"request_digest":20000,"response_content_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"endpoint":20000,"key":20000,"principal_id":20000,"request_digest":20000,"response_content_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."intro_request" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"decision_reason":2000,"fallback_policy":20000,"forwardable_note":2000,"internal_reason":2000,"note_generated_by":20000,"origin":20000,"recorded_channel":20000,"route_type":20000,"status":20000,"value_for_target":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"decision_reason":2000,"fallback_policy":20000,"forwardable_note":2000,"internal_reason":2000,"note_generated_by":20000,"origin":20000,"recorded_channel":20000,"route_type":20000,"status":20000,"value_for_target":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."intro_request" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"decision_reason":2000,"fallback_policy":20000,"forwardable_note":2000,"internal_reason":2000,"note_generated_by":20000,"origin":20000,"recorded_channel":20000,"route_type":20000,"status":20000,"value_for_target":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"decision_reason":2000,"fallback_policy":20000,"forwardable_note":2000,"internal_reason":2000,"note_generated_by":20000,"origin":20000,"recorded_channel":20000,"route_type":20000,"status":20000,"value_for_target":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."knowledge_corpus" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"description":20000,"managed_source":20000,"name":120,"topic_statement":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"description":20000,"managed_source":20000,"name":120,"topic_statement":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."knowledge_corpus" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"description":20000,"managed_source":20000,"name":120,"topic_statement":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"description":20000,"managed_source":20000,"name":120,"topic_statement":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."knowledge_document" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"checksum":20000,"content_type":20000,"filename":20000,"ingest_detail":20000,"ingest_status":20000,"managed_source":20000,"storage_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"checksum":20000,"content_type":20000,"filename":20000,"ingest_detail":20000,"ingest_status":20000,"managed_source":20000,"storage_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."knowledge_document" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"checksum":20000,"content_type":20000,"filename":20000,"ingest_detail":20000,"ingest_status":20000,"managed_source":20000,"storage_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"checksum":20000,"content_type":20000,"filename":20000,"ingest_detail":20000,"ingest_status":20000,"managed_source":20000,"storage_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."lead" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"candidate_company_key":20000,"captured_by":20000,"company_name":120,"disqualify_note":2000,"email":20000,"email_domain_folded":20000,"industry":20000,"linkedin_url":20000,"origin":20000,"origin_detail":500,"partner_accepted_by":20000,"phone":20000,"score_override_reason":2000,"source":20000,"source_id":20000,"source_system":20000,"status":20000,"status_set_by":20000,"title":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"candidate_company_key":20000,"captured_by":20000,"company_name":120,"disqualify_note":2000,"email":20000,"email_domain_folded":20000,"industry":20000,"linkedin_url":20000,"origin":20000,"origin_detail":500,"partner_accepted_by":20000,"phone":20000,"score_override_reason":2000,"source":20000,"source_id":20000,"source_system":20000,"status":20000,"status_set_by":20000,"title":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."lead" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"candidate_company_key":20000,"captured_by":20000,"company_name":120,"disqualify_note":2000,"email":20000,"email_domain_folded":20000,"industry":20000,"linkedin_url":20000,"origin":20000,"origin_detail":500,"partner_accepted_by":20000,"phone":20000,"score_override_reason":2000,"source":20000,"source_id":20000,"source_system":20000,"status":20000,"status_set_by":20000,"title":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"candidate_company_key":20000,"captured_by":20000,"company_name":120,"disqualify_note":2000,"email":20000,"email_domain_folded":20000,"industry":20000,"linkedin_url":20000,"origin":20000,"origin_detail":500,"partner_accepted_by":20000,"phone":20000,"score_override_reason":2000,"source":20000,"source_id":20000,"source_system":20000,"status":20000,"status_set_by":20000,"title":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."lead_disqualify_reason" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"label":120,"shipped_label":120,"shipped_label_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"label":120,"shipped_label":120,"shipped_label_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."lead_disqualify_reason" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"label":120,"shipped_label":120,"shipped_label_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"label":120,"shipped_label":120,"shipped_label_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."lead_manual_signal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"band":20000,"factor":20000,"reason":2000,"signal_kind":20000,"superseded_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"band":20000,"factor":20000,"reason":2000,"signal_kind":20000,"superseded_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."lead_manual_signal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"band":20000,"factor":20000,"reason":2000,"signal_kind":20000,"superseded_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"band":20000,"factor":20000,"reason":2000,"signal_kind":20000,"superseded_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."lead_origin" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"intent":20000,"key":20000,"label":120,"shipped_label":120,"shipped_label_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"intent":20000,"key":20000,"label":120,"shipped_label":120,"shipped_label_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."lead_origin" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"intent":20000,"key":20000,"label":120,"shipped_label":120,"shipped_label_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"intent":20000,"key":20000,"label":120,"shipped_label":120,"shipped_label_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."lead_privacy_notice" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"failure":20000,"locale":20000,"state":20000,"template_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"failure":20000,"locale":20000,"state":20000,"template_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."lead_privacy_notice" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"failure":20000,"locale":20000,"state":20000,"template_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"failure":20000,"locale":20000,"state":20000,"template_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."lead_score_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"override_reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"override_reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."lead_score_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"override_reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"override_reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."lead_source" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"intent":20000,"key":20000,"label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"intent":20000,"key":20000,"label":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."lead_source" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"intent":20000,"key":20000,"label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"intent":20000,"key":20000,"label":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."legal_entity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"address_city":20000,"address_line1":500,"address_line2":500,"address_postal_code":20000,"address_region":20000,"bank_account_holder":20000,"bic":20000,"captured_by":20000,"country":20000,"default_language":20000,"iban":20000,"icp":20000,"imprint":20000,"legal_name":500,"logo_object_key":20000,"offer_summary":20000,"register_court":20000,"register_number":20000,"signature_template":20000,"usp":20000,"value_proposition":20000,"vat_id":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address_city":20000,"address_line1":500,"address_line2":500,"address_postal_code":20000,"address_region":20000,"bank_account_holder":20000,"bic":20000,"captured_by":20000,"country":20000,"default_language":20000,"iban":20000,"icp":20000,"imprint":20000,"legal_name":500,"logo_object_key":20000,"offer_summary":20000,"register_court":20000,"register_number":20000,"signature_template":20000,"usp":20000,"value_proposition":20000,"vat_id":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."legal_entity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"address_city":20000,"address_line1":500,"address_line2":500,"address_postal_code":20000,"address_region":20000,"bank_account_holder":20000,"bic":20000,"captured_by":20000,"country":20000,"default_language":20000,"iban":20000,"icp":20000,"imprint":20000,"legal_name":500,"logo_object_key":20000,"offer_summary":20000,"register_court":20000,"register_number":20000,"signature_template":20000,"usp":20000,"value_proposition":20000,"vat_id":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address_city":20000,"address_line1":500,"address_line2":500,"address_postal_code":20000,"address_region":20000,"bank_account_holder":20000,"bic":20000,"captured_by":20000,"country":20000,"default_language":20000,"iban":20000,"icp":20000,"imprint":20000,"legal_name":500,"logo_object_key":20000,"offer_summary":20000,"register_court":20000,"register_number":20000,"signature_template":20000,"usp":20000,"value_proposition":20000,"vat_id":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."legal_entity_member" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"assigned_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"assigned_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."legal_entity_member" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"assigned_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"assigned_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."legal_review" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"comments":2000,"note":2000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"comments":2000,"note":2000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."legal_review" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"comments":2000,"note":2000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"comments":2000,"note":2000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."linkedin_account" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"profile_url":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"profile_url":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."linkedin_account" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"profile_url":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"profile_url":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."linkedin_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"company_name":120,"email":20000,"match_status":20000,"normalized_company":20000,"normalized_name":120,"position":20000,"profile_url":20000,"provider_member_ref":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"company_name":120,"email":20000,"match_status":20000,"normalized_company":20000,"normalized_name":120,"position":20000,"profile_url":20000,"provider_member_ref":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."linkedin_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"company_name":120,"email":20000,"match_status":20000,"normalized_company":20000,"normalized_name":120,"position":20000,"profile_url":20000,"provider_member_ref":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"company_name":120,"email":20000,"match_status":20000,"normalized_company":20000,"normalized_name":120,"position":20000,"profile_url":20000,"provider_member_ref":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."list" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"entity_type":20000,"list_type":20000,"name":120,"purpose":20000,"sharing":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"entity_type":20000,"list_type":20000,"name":120,"purpose":20000,"sharing":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."list" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"entity_type":20000,"list_type":20000,"name":120,"purpose":20000,"sharing":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"entity_type":20000,"list_type":20000,"name":120,"purpose":20000,"sharing":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."list_evaluation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."list_evaluation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."list_live_member" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"entity_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"entity_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."list_live_member" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"entity_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"entity_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."list_member" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"added_by":20000,"entity_type":20000,"note":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"added_by":20000,"entity_type":20000,"note":2000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."list_member" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"added_by":20000,"entity_type":20000,"note":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"added_by":20000,"entity_type":20000,"note":2000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."list_member_event" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"action":20000,"actor":20000,"entity_type":20000,"member_note":2000,"note":2000,"reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"action":20000,"actor":20000,"entity_type":20000,"member_note":2000,"note":2000,"reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."list_member_event" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"action":20000,"actor":20000,"entity_type":20000,"member_note":2000,"note":2000,"reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"action":20000,"actor":20000,"entity_type":20000,"member_note":2000,"note":2000,"reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."list_revision" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"changed_by":20000,"name":120,"purpose":20000,"sharing":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"changed_by":20000,"name":120,"purpose":20000,"sharing":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."list_revision" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"changed_by":20000,"name":120,"purpose":20000,"sharing":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"changed_by":20000,"name":120,"purpose":20000,"sharing":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."mail_draft" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"anchor_type":20000,"bcc_addresses":20000,"cc_addresses":20000,"communication_context":20000,"scheduled_tz":20000,"subject":20000,"to_addresses":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"anchor_type":20000,"bcc_addresses":20000,"cc_addresses":20000,"communication_context":20000,"scheduled_tz":20000,"subject":20000,"to_addresses":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."mail_draft" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"anchor_type":20000,"bcc_addresses":20000,"cc_addresses":20000,"communication_context":20000,"scheduled_tz":20000,"subject":20000,"to_addresses":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"anchor_type":20000,"bcc_addresses":20000,"cc_addresses":20000,"communication_context":20000,"scheduled_tz":20000,"subject":20000,"to_addresses":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."maskable_field" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"field":20000,"object":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"field":20000,"object":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."maskable_field" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"field":20000,"object":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"field":20000,"object":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."meeting_calendar_episode" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"provider":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"provider":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."meeting_calendar_episode" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"provider":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"provider":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."meeting_invitation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"attention_reason":2000,"calendar_id":20000,"command":20000,"management_hash":20000,"management_ref":20000,"provider":20000,"reminder_status":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"attention_reason":2000,"calendar_id":20000,"command":20000,"management_hash":20000,"management_ref":20000,"provider":20000,"reminder_status":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."meeting_invitation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"attention_reason":2000,"calendar_id":20000,"command":20000,"management_hash":20000,"management_ref":20000,"provider":20000,"reminder_status":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"attention_reason":2000,"calendar_id":20000,"command":20000,"management_hash":20000,"management_ref":20000,"provider":20000,"reminder_status":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."meeting_proposal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"link_ref":20000,"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"link_ref":20000,"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."meeting_proposal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"link_ref":20000,"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"link_ref":20000,"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."meeting_readiness" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"agenda_fingerprint":20000,"captured_by":20000,"participants_fingerprint":20000,"revision":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"agenda_fingerprint":20000,"captured_by":20000,"participants_fingerprint":20000,"revision":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."meeting_readiness" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"agenda_fingerprint":20000,"captured_by":20000,"participants_fingerprint":20000,"revision":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"agenda_fingerprint":20000,"captured_by":20000,"participants_fingerprint":20000,"revision":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."mfa_challenge_spent" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"jti":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"jti":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."mfa_challenge_spent" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"jti":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"jti":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."mfa_recovery_code" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"code_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"code_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."mfa_recovery_code" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"code_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"code_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."notice" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"body":2000,"captured_by":20000,"dedupe_key":20000,"email_error":500,"kind":20000,"subject":500,"target_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"body":2000,"captured_by":20000,"dedupe_key":20000,"email_error":500,"kind":20000,"subject":500,"target_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."notice" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"body":2000,"captured_by":20000,"dedupe_key":20000,"email_error":500,"kind":20000,"subject":500,"target_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"body":2000,"captured_by":20000,"dedupe_key":20000,"email_error":500,"kind":20000,"subject":500,"target_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."notification_digest_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"mail_error":500}')) EXECUTE FUNCTION public.refuse_oversized_text('{"mail_error":500}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."notification_digest_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"mail_error":500}')) EXECUTE FUNCTION public.refuse_oversized_text('{"mail_error":500}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."notification_preference" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"class":20000,"delivery":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"class":20000,"delivery":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."notification_preference" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"class":20000,"delivery":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"class":20000,"delivery":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."oauth_authorization_code" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"client_id":20000,"code_challenge":20000,"code_hash":20000,"redirect_uri":20000,"resource":20000,"scopes":20000,"work_areas":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"client_id":20000,"code_challenge":20000,"code_hash":20000,"redirect_uri":20000,"resource":20000,"scopes":20000,"work_areas":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."oauth_authorization_code" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"client_id":20000,"code_challenge":20000,"code_hash":20000,"redirect_uri":20000,"resource":20000,"scopes":20000,"work_areas":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"client_id":20000,"code_challenge":20000,"code_hash":20000,"redirect_uri":20000,"resource":20000,"scopes":20000,"work_areas":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."oauth_client" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"client_id":20000,"client_name":120,"client_secret_hash":20000,"created_via":20000,"redirect_uris":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"client_id":20000,"client_name":120,"client_secret_hash":20000,"created_via":20000,"redirect_uris":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."oauth_client" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"client_id":20000,"client_name":120,"client_secret_hash":20000,"created_via":20000,"redirect_uris":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"client_id":20000,"client_name":120,"client_secret_hash":20000,"created_via":20000,"redirect_uris":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."oauth_grant" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"client_id":20000,"resource":20000,"scopes":20000,"work_areas":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"client_id":20000,"resource":20000,"scopes":20000,"work_areas":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."oauth_grant" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"client_id":20000,"resource":20000,"scopes":20000,"work_areas":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"client_id":20000,"resource":20000,"scopes":20000,"work_areas":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."oauth_refresh_token" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."oauth_refresh_token" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."offer" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"intro_text":20000,"language":20000,"offer_number":20000,"pdf_asset_ref":20000,"source":20000,"status":20000,"terms_text":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"intro_text":20000,"language":20000,"offer_number":20000,"pdf_asset_ref":20000,"source":20000,"status":20000,"terms_text":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."offer" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"intro_text":20000,"language":20000,"offer_number":20000,"pdf_asset_ref":20000,"source":20000,"status":20000,"terms_text":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"intro_text":20000,"language":20000,"offer_number":20000,"pdf_asset_ref":20000,"source":20000,"status":20000,"terms_text":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."offer_line_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"billing_model":20000,"description":20000,"proposal_state":20000,"tax_category":20000,"unit":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"billing_model":20000,"description":20000,"proposal_state":20000,"tax_category":20000,"unit":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."offer_line_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"billing_model":20000,"description":20000,"proposal_state":20000,"tax_category":20000,"unit":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"billing_model":20000,"description":20000,"proposal_state":20000,"tax_category":20000,"unit":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."offer_template" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"locale":20000,"name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"locale":20000,"name":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."offer_template" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"locale":20000,"name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"locale":20000,"name":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."onboarding_wizard_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"path":20000,"selected_fact_keys":20000,"source_mode":20000,"step":20000,"website_url":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"path":20000,"selected_fact_keys":20000,"source_mode":20000,"step":20000,"website_url":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."onboarding_wizard_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"path":20000,"selected_fact_keys":20000,"source_mode":20000,"step":20000,"website_url":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"path":20000,"selected_fact_keys":20000,"source_mode":20000,"step":20000,"website_url":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."partner" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"activated_by":20000,"activation_basis":20000,"captured_by":20000,"cert_status":20000,"margin_tier":20000,"next_step":20000,"partner_fit_override_reason":2000,"partner_kinds":20000,"relationship_stage":20000,"served_segments":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"activated_by":20000,"activation_basis":20000,"captured_by":20000,"cert_status":20000,"margin_tier":20000,"next_step":20000,"partner_fit_override_reason":2000,"partner_kinds":20000,"relationship_stage":20000,"served_segments":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."partner" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"activated_by":20000,"activation_basis":20000,"captured_by":20000,"cert_status":20000,"margin_tier":20000,"next_step":20000,"partner_fit_override_reason":2000,"partner_kinds":20000,"relationship_stage":20000,"served_segments":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"activated_by":20000,"activation_basis":20000,"captured_by":20000,"cert_status":20000,"margin_tier":20000,"next_step":20000,"partner_fit_override_reason":2000,"partner_kinds":20000,"relationship_stage":20000,"served_segments":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."partner_stage" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"key":20000,"label":120,"meaning":20000,"shipped_label":120,"shipped_label_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key":20000,"label":120,"meaning":20000,"shipped_label":120,"shipped_label_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."partner_stage" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"key":20000,"label":120,"meaning":20000,"shipped_label":120,"shipped_label_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key":20000,"label":120,"meaning":20000,"shipped_label":120,"shipped_label_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."passport" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"label":120,"scopes":20000,"token_hash":20000,"work_areas":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"label":120,"scopes":20000,"token_hash":20000,"work_areas":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."passport" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"label":120,"scopes":20000,"token_hash":20000,"work_areas":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"label":120,"scopes":20000,"token_hash":20000,"work_areas":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."pipeline" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"name":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."pipeline" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"name":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."preference_token" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"revoked_reason":2000,"token":20000,"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"revoked_reason":2000,"token":20000,"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."preference_token" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"revoked_reason":2000,"token":20000,"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"revoked_reason":2000,"token":20000,"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."privacy_notice_case" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"allowed_routes":20000,"blocked_reason":2000,"resolution_note":2000,"rule":20000,"source_evidence_kind":20000,"source_kind":20000,"source_review_note":2000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"allowed_routes":20000,"blocked_reason":2000,"resolution_note":2000,"rule":20000,"source_evidence_kind":20000,"source_kind":20000,"source_review_note":2000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."privacy_notice_case" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"allowed_routes":20000,"blocked_reason":2000,"resolution_note":2000,"rule":20000,"source_evidence_kind":20000,"source_kind":20000,"source_review_note":2000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"allowed_routes":20000,"blocked_reason":2000,"resolution_note":2000,"rule":20000,"source_evidence_kind":20000,"source_kind":20000,"source_review_note":2000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."privacy_request_proof" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"completed_by_id":20000,"completed_by_type":20000,"erased_uncounted":20000,"identity_check":20000,"request_kind":20000,"request_number":20000,"requester_email":20000,"result":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"completed_by_id":20000,"completed_by_type":20000,"erased_uncounted":20000,"identity_check":20000,"request_kind":20000,"request_number":20000,"requester_email":20000,"result":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."privacy_request_proof" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"completed_by_id":20000,"completed_by_type":20000,"erased_uncounted":20000,"identity_check":20000,"request_kind":20000,"request_number":20000,"requester_email":20000,"result":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"completed_by_id":20000,"completed_by_type":20000,"erased_uncounted":20000,"identity_check":20000,"request_kind":20000,"request_number":20000,"requester_email":20000,"result":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."product" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"billing_model":20000,"captured_by":20000,"component_kind":20000,"description":20000,"name":120,"sku":20000,"source":20000,"tax_category":20000,"unit":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"billing_model":20000,"captured_by":20000,"component_kind":20000,"description":20000,"name":120,"sku":20000,"source":20000,"tax_category":20000,"unit":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."product" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"billing_model":20000,"captured_by":20000,"component_kind":20000,"description":20000,"name":120,"sku":20000,"source":20000,"tax_category":20000,"unit":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"billing_model":20000,"captured_by":20000,"component_kind":20000,"description":20000,"name":120,"sku":20000,"source":20000,"tax_category":20000,"unit":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."product_translation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"description":20000,"language":20000,"name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"description":20000,"language":20000,"name":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."product_translation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"description":20000,"language":20000,"name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"description":20000,"language":20000,"name":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."project" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"closed_reason":2000,"description":20000,"key":20000,"name":120,"phase":20000,"source":20000,"source_system":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"closed_reason":2000,"description":20000,"key":20000,"name":120,"phase":20000,"source":20000,"source_system":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."project" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"closed_reason":2000,"description":20000,"key":20000,"name":120,"phase":20000,"source":20000,"source_system":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"closed_reason":2000,"description":20000,"key":20000,"name":120,"phase":20000,"source":20000,"source_system":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."project_health_assessment" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"note":2000,"source":20000,"source_author":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"note":2000,"source":20000,"source_author":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."project_health_assessment" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"note":2000,"source":20000,"source_author":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"note":2000,"source":20000,"source_author":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."project_phase_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"changed_by":20000,"from_phase":20000,"reason":2000,"to_phase":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"changed_by":20000,"from_phase":20000,"reason":2000,"to_phase":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."project_phase_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"changed_by":20000,"from_phase":20000,"reason":2000,"to_phase":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"changed_by":20000,"from_phase":20000,"reason":2000,"to_phase":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."provider_applied_field" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"applied_value":20000,"captured_by":20000,"provider":20000,"target_field":20000,"target_table":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"applied_value":20000,"captured_by":20000,"provider":20000,"target_field":20000,"target_table":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."provider_applied_field" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"applied_value":20000,"captured_by":20000,"provider":20000,"target_field":20000,"target_table":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"applied_value":20000,"captured_by":20000,"provider":20000,"target_field":20000,"target_table":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."provider_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"categories":20000,"credential_ref":20000,"last_safe_status_code":20000,"mode":20000,"preset":20000,"provider":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"categories":20000,"credential_ref":20000,"last_safe_status_code":20000,"mode":20000,"preset":20000,"provider":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."provider_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"categories":20000,"credential_ref":20000,"last_safe_status_code":20000,"mode":20000,"preset":20000,"provider":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"categories":20000,"credential_ref":20000,"last_safe_status_code":20000,"mode":20000,"preset":20000,"provider":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."provider_connection_budget" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"pool":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"pool":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."provider_connection_budget" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"pool":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"pool":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."provider_employment_resolution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"episode_key":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"episode_key":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."provider_employment_resolution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"episode_key":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"episode_key":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."provider_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"input_fingerprint":20000,"last_safe_status_code":20000,"provider":20000,"provider_job_id":20000,"requested_categories":20000,"skip_reason":2000,"state":20000,"subject_kind":20000,"trigger":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"input_fingerprint":20000,"last_safe_status_code":20000,"provider":20000,"provider_job_id":20000,"requested_categories":20000,"skip_reason":2000,"state":20000,"subject_kind":20000,"trigger":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."provider_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"input_fingerprint":20000,"last_safe_status_code":20000,"provider":20000,"provider_job_id":20000,"requested_categories":20000,"skip_reason":2000,"state":20000,"subject_kind":20000,"trigger":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"input_fingerprint":20000,"last_safe_status_code":20000,"provider":20000,"provider_job_id":20000,"requested_categories":20000,"skip_reason":2000,"state":20000,"subject_kind":20000,"trigger":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."provider_run_category" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"category":20000,"provider":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"category":20000,"provider":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."provider_run_category" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"category":20000,"provider":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"category":20000,"provider":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."provider_run_reservation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"pool":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"pool":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."provider_run_reservation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"pool":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"pool":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."record_assignment" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."record_assignment" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."record_grant" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"access":20000,"reason":2000,"record_type":20000,"subject_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"access":20000,"reason":2000,"record_type":20000,"subject_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."record_grant" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"access":20000,"reason":2000,"record_type":20000,"subject_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"access":20000,"reason":2000,"record_type":20000,"subject_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."record_link" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"from_type":20000,"link_type":20000,"to_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"from_type":20000,"link_type":20000,"to_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."record_link" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"from_type":20000,"link_type":20000,"to_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"from_type":20000,"link_type":20000,"to_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."record_role" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"assignee_kinds":20000,"key":20000,"label":120,"record_types":20000,"shipped_label":120,"shipped_label_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"assignee_kinds":20000,"key":20000,"label":120,"record_types":20000,"shipped_label":120,"shipped_label_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."record_role" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"assignee_kinds":20000,"key":20000,"label":120,"record_types":20000,"shipped_label":120,"shipped_label_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"assignee_kinds":20000,"key":20000,"label":120,"record_types":20000,"shipped_label":120,"shipped_label_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."relationship" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"employment_status":20000,"ended_precision":20000,"kind":20000,"role":20000,"source":20000,"started_precision":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"employment_status":20000,"ended_precision":20000,"kind":20000,"role":20000,"source":20000,"started_precision":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."relationship" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"employment_status":20000,"ended_precision":20000,"kind":20000,"role":20000,"source":20000,"started_precision":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"employment_status":20000,"ended_precision":20000,"kind":20000,"role":20000,"source":20000,"started_precision":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."relationship_nudge_dismissal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"set_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"set_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."relationship_nudge_dismissal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"set_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"set_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."report_definition" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"audience":20000,"description":20000,"name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"audience":20000,"description":20000,"name":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."report_definition" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"audience":20000,"description":20000,"name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"audience":20000,"description":20000,"name":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."report_edition" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"publication_audience":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"publication_audience":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."report_edition" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"publication_audience":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"publication_audience":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."report_edition_contribution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"context_id":20000,"contribution_key":20000,"group_key":20000,"metric":20000,"source_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"context_id":20000,"contribution_key":20000,"group_key":20000,"metric":20000,"source_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."report_edition_contribution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"context_id":20000,"contribution_key":20000,"group_key":20000,"metric":20000,"source_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"context_id":20000,"contribution_key":20000,"group_key":20000,"metric":20000,"source_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."report_edition_question" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"card_id":20000,"entity":20000,"reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"card_id":20000,"entity":20000,"reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."report_edition_question" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"card_id":20000,"entity":20000,"reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"card_id":20000,"entity":20000,"reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."report_edition_question_source" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"card_id":20000,"source_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"card_id":20000,"source_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."report_edition_question_source" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"card_id":20000,"source_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"card_id":20000,"source_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."report_execution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"execution_key":20000,"reason":2000,"request_hash":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"execution_key":20000,"reason":2000,"request_hash":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."report_execution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"execution_key":20000,"reason":2000,"request_hash":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"execution_key":20000,"reason":2000,"request_hash":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."report_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"schema_version":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"schema_version":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."report_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"schema_version":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"schema_version":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."report_schedule" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"last_status":20000,"timezone":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"last_status":20000,"timezone":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."report_schedule" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"last_status":20000,"timezone":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"last_status":20000,"timezone":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."restore_drill" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"notes":2000,"operator":20000,"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"notes":2000,"operator":20000,"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."restore_drill" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"notes":2000,"operator":20000,"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"notes":2000,"operator":20000,"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."retention_policy" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"action":20000,"category":20000,"lawful_basis":20000,"object_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"action":20000,"category":20000,"lawful_basis":20000,"object_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."retention_policy" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"action":20000,"category":20000,"lawful_basis":20000,"object_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"action":20000,"category":20000,"lawful_basis":20000,"object_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."role" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"key":20000,"name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key":20000,"name":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."role" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"key":20000,"name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key":20000,"name":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."runner_job" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"agent_spec":20000,"last_error":20000,"status":20000,"trigger_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"agent_spec":20000,"last_error":20000,"status":20000,"trigger_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."runner_job" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"agent_spec":20000,"last_error":20000,"status":20000,"trigger_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"agent_spec":20000,"last_error":20000,"status":20000,"trigger_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."sales_goal_plan_revision" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"kind":20000,"origin":20000,"reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000,"origin":20000,"reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."sales_goal_plan_revision" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"kind":20000,"origin":20000,"reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000,"origin":20000,"reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."sales_initiative" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"description":20000,"name":120,"objective_metric":20000,"source":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"description":20000,"name":120,"objective_metric":20000,"source":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."sales_initiative" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"description":20000,"name":120,"objective_metric":20000,"source":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"description":20000,"name":120,"objective_metric":20000,"source":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."sales_initiative_baseline" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."sales_initiative_baseline" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."sales_initiative_enrolment" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"reason":20000,"record_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"reason":20000,"record_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."sales_initiative_enrolment" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"reason":20000,"record_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"reason":20000,"record_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."sales_initiative_member" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"reason":2000,"record_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"reason":2000,"record_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."sales_initiative_member" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"reason":2000,"record_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"reason":2000,"record_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."sales_initiative_revision" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"created_by":20000,"reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"created_by":20000,"reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."sales_initiative_revision" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"created_by":20000,"reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"created_by":20000,"reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."sales_target" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"metric":20000,"period_kind":20000,"scope_kind":20000,"unit":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"metric":20000,"period_kind":20000,"scope_kind":20000,"unit":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."sales_target" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"metric":20000,"period_kind":20000,"scope_kind":20000,"unit":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"metric":20000,"period_kind":20000,"scope_kind":20000,"unit":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."saved_view" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"name":120,"resource":20000,"shared_scope":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"name":120,"resource":20000,"shared_scope":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."saved_view" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"name":120,"resource":20000,"shared_scope":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"name":120,"resource":20000,"shared_scope":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."scheduled_send" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"agent_actor_id":20000,"held_reason":2000,"origin_kind":20000,"principal_kind":20000,"scheduled_tz":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"agent_actor_id":20000,"held_reason":2000,"origin_kind":20000,"principal_kind":20000,"scheduled_tz":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."scheduled_send" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"agent_actor_id":20000,"held_reason":2000,"origin_kind":20000,"principal_kind":20000,"scheduled_tz":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"agent_actor_id":20000,"held_reason":2000,"origin_kind":20000,"principal_kind":20000,"scheduled_tz":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."sdr_handoff" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"note":2000,"reason_applies_to":2000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"note":2000,"reason_applies_to":2000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."sdr_handoff" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"note":2000,"reason_applies_to":2000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"note":2000,"reason_applies_to":2000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."sdr_handoff_event" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"actor":20000,"note":2000,"reason_applies_to":2000,"to_status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"actor":20000,"note":2000,"reason_applies_to":2000,"to_status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."sdr_handoff_event" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"actor":20000,"note":2000,"reason_applies_to":2000,"to_status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"actor":20000,"note":2000,"reason_applies_to":2000,"to_status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."sdr_handoff_reason" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"applies_to":20000,"label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"applies_to":20000,"label":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."sdr_handoff_reason" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"applies_to":20000,"label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"applies_to":20000,"label":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."session" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"token_hash":20000,"user_agent":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"token_hash":20000,"user_agent":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."session" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"token_hash":20000,"user_agent":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"token_hash":20000,"user_agent":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."setting" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."setting" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."setup_token" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."setup_token" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."signal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"entity_type":20000,"fingerprint":20000,"kind":20000,"raw_ref":20000,"resolution_state":20000,"severity":20000,"source":20000,"source_channel":20000,"status":20000,"summary":500,"visibility":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"entity_type":20000,"fingerprint":20000,"kind":20000,"raw_ref":20000,"resolution_state":20000,"severity":20000,"source":20000,"source_channel":20000,"status":20000,"summary":500,"visibility":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."signal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"entity_type":20000,"fingerprint":20000,"kind":20000,"raw_ref":20000,"resolution_state":20000,"severity":20000,"source":20000,"source_channel":20000,"status":20000,"summary":500,"visibility":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"entity_type":20000,"fingerprint":20000,"kind":20000,"raw_ref":20000,"resolution_state":20000,"severity":20000,"source":20000,"source_channel":20000,"status":20000,"summary":500,"visibility":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."signal_resolution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"matched_on":20000,"note":2000,"outcome":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"matched_on":20000,"note":2000,"outcome":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."signal_resolution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"matched_on":20000,"note":2000,"outcome":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"matched_on":20000,"note":2000,"outcome":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."signal_thread_scan" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"thread_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"thread_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."signal_thread_scan" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"thread_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"thread_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."site_read" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"logo_icon_object_key":20000,"logo_icon_origin":20000,"logo_object_key":20000,"logo_origin":20000,"phase":20000,"proposal_hash":20000,"requested_by":20000,"seed_url":20000,"status":20000,"status_code":20000,"status_detail":20000,"stopped_reason":2000,"target_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"logo_icon_object_key":20000,"logo_icon_origin":20000,"logo_object_key":20000,"logo_origin":20000,"phase":20000,"proposal_hash":20000,"requested_by":20000,"seed_url":20000,"status":20000,"status_code":20000,"status_detail":20000,"stopped_reason":2000,"target_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."site_read" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"logo_icon_object_key":20000,"logo_icon_origin":20000,"logo_object_key":20000,"logo_origin":20000,"phase":20000,"proposal_hash":20000,"requested_by":20000,"seed_url":20000,"status":20000,"status_code":20000,"status_detail":20000,"stopped_reason":2000,"target_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"logo_icon_object_key":20000,"logo_icon_origin":20000,"logo_object_key":20000,"logo_origin":20000,"phase":20000,"proposal_hash":20000,"requested_by":20000,"seed_url":20000,"status":20000,"status_code":20000,"status_detail":20000,"stopped_reason":2000,"target_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."stage" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"name":120,"semantic":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"name":120,"semantic":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."stage" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"name":120,"semantic":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"name":120,"semantic":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."stage_exit_criterion" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"hint":500,"key":20000,"kind":20000,"label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"hint":500,"key":20000,"kind":20000,"label":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."stage_exit_criterion" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"hint":500,"key":20000,"kind":20000,"label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"hint":500,"key":20000,"kind":20000,"label":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."stage_progression_outcome" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"evidence_kinds":20000,"outcome":20000,"rejection_reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"evidence_kinds":20000,"outcome":20000,"rejection_reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."stage_progression_outcome" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"evidence_kinds":20000,"outcome":20000,"rejection_reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"evidence_kinds":20000,"outcome":20000,"rejection_reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."stage_progression_policy" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"mode":20000,"suspended_reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"mode":20000,"suspended_reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."stage_progression_policy" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"mode":20000,"suspended_reason":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"mode":20000,"suspended_reason":2000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."stored_object_intent" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"kind":20000,"storage_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000,"storage_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."stored_object_intent" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"kind":20000,"storage_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000,"storage_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."suggestion_dismissal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"fingerprint":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."suggestion_dismissal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"fingerprint":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."system_log" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"action":20000,"actor_id":20000,"actor_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"action":20000,"actor_id":20000,"actor_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."system_log" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"action":20000,"actor_id":20000,"actor_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"action":20000,"actor_id":20000,"actor_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."tag" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"color":20000,"description":20000,"name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"color":20000,"description":20000,"name":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."tag" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"color":20000,"description":20000,"name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"color":20000,"description":20000,"name":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."tag_interest_read" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"content_digest":20000,"vocabulary_digest":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"content_digest":20000,"vocabulary_digest":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."tag_interest_read" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"content_digest":20000,"vocabulary_digest":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"content_digest":20000,"vocabulary_digest":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."tag_suggestion" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."tag_suggestion" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."taggable" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"assigned_by_kind":20000,"entity_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"assigned_by_kind":20000,"entity_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."taggable" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"assigned_by_kind":20000,"entity_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"assigned_by_kind":20000,"entity_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."tax_rate" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"category":20000,"country":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"category":20000,"country":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."tax_rate" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"category":20000,"country":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"category":20000,"country":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."team" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"name":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."team" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"name":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."team_weekly_review" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"summary":500,"summary_language":20000,"team_name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"summary":500,"summary_language":20000,"team_name":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."team_weekly_review" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"summary":500,"summary_language":20000,"team_name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"summary":500,"summary_language":20000,"team_name":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."team_weekly_review_coverage" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"absent":20000,"basis":20000,"period_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"absent":20000,"basis":20000,"period_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."team_weekly_review_coverage" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"absent":20000,"basis":20000,"period_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"absent":20000,"basis":20000,"period_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."team_weekly_review_outlook" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"forward_measure":20000,"period_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"forward_measure":20000,"period_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."team_weekly_review_outlook" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"forward_measure":20000,"period_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"forward_measure":20000,"period_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."team_weekly_review_rep" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"focus_kind":20000,"focus_label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"focus_kind":20000,"focus_label":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."team_weekly_review_rep" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"focus_kind":20000,"focus_label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"focus_kind":20000,"focus_label":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."team_weekly_review_rep_funnel" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"next_stage_name":120,"pipeline_name":120,"stage_name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"next_stage_name":120,"pipeline_name":120,"stage_name":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."team_weekly_review_rep_funnel" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"next_stage_name":120,"pipeline_name":120,"stage_name":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"next_stage_name":120,"pipeline_name":120,"stage_name":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."team_weekly_review_rep_pattern" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"kind":20000,"label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000,"label":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."team_weekly_review_rep_pattern" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"kind":20000,"label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000,"label":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."technical_lookup_cache" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"query":20000,"record_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"query":20000,"record_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."technical_lookup_cache" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"query":20000,"record_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"query":20000,"record_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."transcript_read" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"requested_by":20000,"status":20000,"status_detail":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"requested_by":20000,"status":20000,"status_detail":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."transcript_read" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"requested_by":20000,"status":20000,"status_detail":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"requested_by":20000,"status":20000,"status_detail":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."user_mfa" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"secret_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"secret_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."user_mfa" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"secret_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"secret_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."user_record_view" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"entity_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"entity_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."user_record_view" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"entity_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"entity_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."vault_secret" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."vault_secret" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."voice_build" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"candidate_action":20000,"reason":2000,"source_hash":20000,"stage":20000,"status":20000,"status_code":20000,"status_detail":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"candidate_action":20000,"reason":2000,"source_hash":20000,"stage":20000,"status":20000,"status_code":20000,"status_detail":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."voice_build" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"candidate_action":20000,"reason":2000,"source_hash":20000,"stage":20000,"status":20000,"status_code":20000,"status_detail":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"candidate_action":20000,"reason":2000,"source_hash":20000,"stage":20000,"status":20000,"status_code":20000,"status_detail":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."voice_corpus_source" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"content_hash":20000,"exclusion_reason":2000,"extractor_version":20000,"kind":20000,"origin":20000,"register":20000,"source":20000,"source_label":500,"source_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"content_hash":20000,"exclusion_reason":2000,"extractor_version":20000,"kind":20000,"origin":20000,"register":20000,"source":20000,"source_label":500,"source_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."voice_corpus_source" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"content_hash":20000,"exclusion_reason":2000,"extractor_version":20000,"kind":20000,"origin":20000,"register":20000,"source":20000,"source_label":500,"source_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"content_hash":20000,"exclusion_reason":2000,"extractor_version":20000,"kind":20000,"origin":20000,"register":20000,"source":20000,"source_label":500,"source_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."voice_learning_signal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"final_captured_by":20000,"final_text":20000,"generated_original":20000,"outcome":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"final_captured_by":20000,"final_text":20000,"generated_original":20000,"outcome":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."voice_learning_signal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"final_captured_by":20000,"final_text":20000,"generated_original":20000,"outcome":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"final_captured_by":20000,"final_text":20000,"generated_original":20000,"outcome":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."voice_profile" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"active_source_hash":20000,"captured_by":20000,"model_ref":20000,"personality_md":20000,"scope":20000,"source":20000,"status":20000,"voice_profile_md":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"active_source_hash":20000,"captured_by":20000,"model_ref":20000,"personality_md":20000,"scope":20000,"source":20000,"status":20000,"voice_profile_md":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."voice_profile" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"active_source_hash":20000,"captured_by":20000,"model_ref":20000,"personality_md":20000,"scope":20000,"source":20000,"status":20000,"voice_profile_md":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"active_source_hash":20000,"captured_by":20000,"model_ref":20000,"personality_md":20000,"scope":20000,"source":20000,"status":20000,"voice_profile_md":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."voice_profile_delta" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"activation_outcome":20000,"classification":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"activation_outcome":20000,"classification":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."voice_profile_delta" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"activation_outcome":20000,"classification":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"activation_outcome":20000,"classification":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."voice_profile_version" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"activation_policy_version":20000,"builder_version":20000,"captured_by":20000,"model_name":120,"model_provider":20000,"reason":2000,"review_reasons":2000,"source":20000,"source_hash":20000,"status":20000,"voice_profile_md":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"activation_policy_version":20000,"builder_version":20000,"captured_by":20000,"model_name":120,"model_provider":20000,"reason":2000,"review_reasons":2000,"source":20000,"source_hash":20000,"status":20000,"voice_profile_md":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."voice_profile_version" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"activation_policy_version":20000,"builder_version":20000,"captured_by":20000,"model_name":120,"model_provider":20000,"reason":2000,"review_reasons":2000,"source":20000,"source_hash":20000,"status":20000,"voice_profile_md":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"activation_policy_version":20000,"builder_version":20000,"captured_by":20000,"model_name":120,"model_provider":20000,"reason":2000,"review_reasons":2000,"source":20000,"source_hash":20000,"status":20000,"voice_profile_md":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."web_form" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"allowed_origins":20000,"captured_by":20000,"consent_policy_version":20000,"consent_wording":2000,"form_key":20000,"lead_origin":20000,"name":120,"privacy_notice":500,"redirect_url":20000,"secret_ref":20000,"sender":20000,"state":20000,"success_message":500}')) EXECUTE FUNCTION public.refuse_oversized_text('{"allowed_origins":20000,"captured_by":20000,"consent_policy_version":20000,"consent_wording":2000,"form_key":20000,"lead_origin":20000,"name":120,"privacy_notice":500,"redirect_url":20000,"secret_ref":20000,"sender":20000,"state":20000,"success_message":500}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."web_form" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"allowed_origins":20000,"captured_by":20000,"consent_policy_version":20000,"consent_wording":2000,"form_key":20000,"lead_origin":20000,"name":120,"privacy_notice":500,"redirect_url":20000,"secret_ref":20000,"sender":20000,"state":20000,"success_message":500}')) EXECUTE FUNCTION public.refuse_oversized_text('{"allowed_origins":20000,"captured_by":20000,"consent_policy_version":20000,"consent_wording":2000,"form_key":20000,"lead_origin":20000,"name":120,"privacy_notice":500,"redirect_url":20000,"secret_ref":20000,"sender":20000,"state":20000,"success_message":500}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."webhook_delivery" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"content_type":20000,"entity_type":20000,"event_type":20000,"last_error":20000,"payload":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"content_type":20000,"entity_type":20000,"event_type":20000,"last_error":20000,"payload":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."webhook_delivery" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"content_type":20000,"entity_type":20000,"event_type":20000,"last_error":20000,"payload":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"content_type":20000,"entity_type":20000,"event_type":20000,"last_error":20000,"payload":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."webhook_subscription" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"body_content_type":20000,"body_template":20000,"event_types":20000,"signing_secret_ref":20000,"state":20000,"target_url":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"body_content_type":20000,"body_template":20000,"event_types":20000,"signing_secret_ref":20000,"state":20000,"target_url":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."webhook_subscription" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"body_content_type":20000,"body_template":20000,"event_types":20000,"signing_secret_ref":20000,"state":20000,"target_url":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"body_content_type":20000,"body_template":20000,"event_types":20000,"signing_secret_ref":20000,"state":20000,"target_url":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."weekly_plan" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"capacity_note":2000,"captured_by":20000,"risks":2000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"capacity_note":2000,"captured_by":20000,"risks":2000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."weekly_plan" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"capacity_note":2000,"captured_by":20000,"risks":2000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"capacity_note":2000,"captured_by":20000,"risks":2000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."weekly_plan_commitment" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"help_requested":2000,"label":500,"linked_record_type":20000,"manager_response":2000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"help_requested":2000,"label":500,"linked_record_type":20000,"manager_response":2000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."weekly_plan_commitment" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"help_requested":2000,"label":500,"linked_record_type":20000,"manager_response":2000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"help_requested":2000,"label":500,"linked_record_type":20000,"manager_response":2000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."weekly_review" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"learnings_state":20000,"mail_error":500,"narrative":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"learnings_state":20000,"mail_error":500,"narrative":2000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."weekly_review" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"learnings_state":20000,"mail_error":500,"narrative":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"learnings_state":20000,"mail_error":500,"narrative":2000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."weekly_review_deal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"deal_label":120,"outcome":20000,"to_stage_label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"deal_label":120,"outcome":20000,"to_stage_label":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."weekly_review_deal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"deal_label":120,"outcome":20000,"to_stage_label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"deal_label":120,"outcome":20000,"to_stage_label":120}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."weekly_review_driver" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"bar":20000,"deal_label":120,"period_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"bar":20000,"deal_label":120,"period_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."weekly_review_driver" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"bar":20000,"deal_label":120,"period_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"bar":20000,"deal_label":120,"period_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."weekly_review_learning" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"kind":20000,"text":500}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000,"text":500}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."weekly_review_learning" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"kind":20000,"text":500}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000,"text":500}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."weekly_review_learning_citation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"label":120,"subject_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"label":120,"subject_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."weekly_review_learning_citation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"label":120,"subject_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"label":120,"subject_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."weekly_review_movement" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"bar":20000,"period_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"bar":20000,"period_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."weekly_review_movement" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"bar":20000,"period_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"bar":20000,"period_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."weekly_review_outlook" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"forward_measure":20000,"period_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"forward_measure":20000,"period_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."weekly_review_outlook" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"forward_measure":20000,"period_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"forward_measure":20000,"period_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."withdrawal_credential" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"address":20000,"revoked_reason":2000,"scope":20000,"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address":20000,"revoked_reason":2000,"scope":20000,"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."withdrawal_credential" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"address":20000,"revoked_reason":2000,"scope":20000,"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address":20000,"revoked_reason":2000,"scope":20000,"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."workflow_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"error":20000,"handler":20000,"idempotency_key":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"error":20000,"handler":20000,"idempotency_key":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."workflow_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"error":20000,"handler":20000,"idempotency_key":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"error":20000,"handler":20000,"idempotency_key":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."worklist_done" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"row_id":20000,"set_by":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"row_id":20000,"set_by":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."worklist_done" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"row_id":20000,"set_by":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"row_id":20000,"set_by":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."worklist_pin" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"row_id":20000,"set_by":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"row_id":20000,"set_by":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."worklist_pin" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"row_id":20000,"set_by":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"row_id":20000,"set_by":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."worklist_snapshot" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"params_fingerprint":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"params_fingerprint":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."worklist_snapshot" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"params_fingerprint":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"params_fingerprint":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."workspace_email_domain" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"domain":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"domain":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."workspace_email_domain" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"domain":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"domain":20000,"source":20000}');
RESET lock_timeout;
