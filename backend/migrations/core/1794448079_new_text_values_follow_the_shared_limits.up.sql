CREATE FUNCTION public.text_value_exceeds_bound(new_value text, old_value text, character_limit integer)
RETURNS boolean LANGUAGE sql IMMUTABLE PARALLEL SAFE SET search_path = pg_catalog AS $$
SELECT new_value IS NOT NULL AND new_value COLLATE "C" IS DISTINCT FROM old_value COLLATE "C"
 AND char_length(new_value) > character_limit
$$;
CREATE FUNCTION public.text_values_exceed_bound(new_values text[], old_values text[], character_limit integer)
RETURNS boolean LANGUAGE sql IMMUTABLE PARALLEL SAFE SET search_path = pg_catalog AS $$
SELECT EXISTS (SELECT 1 FROM unnest(new_values) AS item(value)
 WHERE char_length(value) > character_limit
 AND NOT EXISTS (SELECT 1 FROM unnest(old_values) AS previous(value)
                 WHERE previous.value COLLATE "C" = item.value COLLATE "C"))
$$;
CREATE FUNCTION public.refuse_oversized_text() RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
 RAISE EXCEPTION 'text exceeds its character limit' USING ERRCODE = '22001';
END
$$;
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."agenda_fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."audience", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."audience_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."body_completeness", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."calendar_method", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."calendar_uid", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."call_outcome", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."capture_label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."capture_label_declined_ruleset", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."channel_provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."counterparty_email", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."deal_topic_digest", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."direction", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."language", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."meeting_status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."occurred_precision", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."origin", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."owed_verdict", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."owed_verdict_declined_ruleset", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."owed_verdict_ruleset", NULL, 20000) OR
public.text_values_exceed_bound(NEW."redacted_fields", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reply_verdict", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reply_verdict_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."restricted_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."retention_class", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_author_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."source_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_system", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject", NULL, 120) OR
public.text_value_exceeds_bound(NEW."thread_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."agenda_fingerprint", OLD."agenda_fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."audience", OLD."audience", 20000) OR
public.text_value_exceeds_bound(NEW."audience_reason", OLD."audience_reason", 2000) OR
public.text_value_exceeds_bound(NEW."body_completeness", OLD."body_completeness", 20000) OR
public.text_value_exceeds_bound(NEW."calendar_method", OLD."calendar_method", 20000) OR
public.text_value_exceeds_bound(NEW."calendar_uid", OLD."calendar_uid", 20000) OR
public.text_value_exceeds_bound(NEW."call_outcome", OLD."call_outcome", 20000) OR
public.text_value_exceeds_bound(NEW."capture_label", OLD."capture_label", 120) OR
public.text_value_exceeds_bound(NEW."capture_label_declined_ruleset", OLD."capture_label_declined_ruleset", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."channel_provider", OLD."channel_provider", 20000) OR
public.text_value_exceeds_bound(NEW."counterparty_email", OLD."counterparty_email", 20000) OR
public.text_value_exceeds_bound(NEW."deal_topic_digest", OLD."deal_topic_digest", 20000) OR
public.text_value_exceeds_bound(NEW."direction", OLD."direction", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."language", OLD."language", 20000) OR
public.text_value_exceeds_bound(NEW."meeting_status", OLD."meeting_status", 20000) OR
public.text_value_exceeds_bound(NEW."occurred_precision", OLD."occurred_precision", 20000) OR
public.text_value_exceeds_bound(NEW."origin", OLD."origin", 20000) OR
public.text_value_exceeds_bound(NEW."owed_verdict", OLD."owed_verdict", 20000) OR
public.text_value_exceeds_bound(NEW."owed_verdict_declined_ruleset", OLD."owed_verdict_declined_ruleset", 20000) OR
public.text_value_exceeds_bound(NEW."owed_verdict_ruleset", OLD."owed_verdict_ruleset", 20000) OR
public.text_values_exceed_bound(NEW."redacted_fields", OLD."redacted_fields", 20000) OR
public.text_value_exceeds_bound(NEW."reply_verdict", OLD."reply_verdict", 20000) OR
public.text_value_exceeds_bound(NEW."reply_verdict_by", OLD."reply_verdict_by", 20000) OR
public.text_value_exceeds_bound(NEW."restricted_reason", OLD."restricted_reason", 2000) OR
public.text_value_exceeds_bound(NEW."retention_class", OLD."retention_class", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."source_author_name", OLD."source_author_name", 120) OR
public.text_value_exceeds_bound(NEW."source_id", OLD."source_id", 20000) OR
public.text_value_exceeds_bound(NEW."source_system", OLD."source_system", 20000) OR
public.text_value_exceeds_bound(NEW."subject", OLD."subject", 120) OR
public.text_value_exceeds_bound(NEW."thread_key", OLD."thread_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_audience_member" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."created_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_audience_member" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."created_by", OLD."created_by", 20000) OR
public.text_value_exceeds_bound(NEW."subject_type", OLD."subject_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_calendar_method_backfill" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."direction", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."outcome", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_calendar_method_backfill" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."direction", OLD."direction", 20000) OR
public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_commitment_admission" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."locator", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."outcome", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."rule_version", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_quote", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."summary", NULL, 500)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_commitment_admission" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."locator", OLD."locator", 20000) OR
public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000) OR
public.text_value_exceeds_bound(NEW."rule_version", OLD."rule_version", 20000) OR
public.text_value_exceeds_bound(NEW."source_quote", OLD."source_quote", 20000) OR
public.text_value_exceeds_bound(NEW."summary", OLD."summary", 500)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_identity" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."attested_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."identity_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."identity_kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_identity" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."attested_by", OLD."attested_by", 20000) OR
public.text_value_exceeds_bound(NEW."identity_key", OLD."identity_key", 20000) OR
public.text_value_exceeds_bound(NEW."identity_kind", OLD."identity_kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_kind" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_kind" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_link" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."entity_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_link" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."entity_type", OLD."entity_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_mail_reference" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."referenced_id", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_mail_reference" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."referenced_id", OLD."referenced_id", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_meeting_attendee_repair" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."outcome", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_meeting_attendee_repair" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_meeting_history" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."actor", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."scheduled_precision", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_system", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_meeting_history" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."actor", OLD."actor", 20000) OR
public.text_value_exceeds_bound(NEW."scheduled_precision", OLD."scheduled_precision", 20000) OR
public.text_value_exceeds_bound(NEW."source_id", OLD."source_id", 20000) OR
public.text_value_exceeds_bound(NEW."source_system", OLD."source_system", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_meeting_rsvp_backfill" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."outcome", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_meeting_rsvp_backfill" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_own_sent_mail_repair" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."outcome", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_own_sent_mail_repair" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_participant" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."address", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."address_domain_folded", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."channel_user_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."display_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."domain_evidence_precision", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."response", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."role", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_participant" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."address", OLD."address", 20000) OR
public.text_value_exceeds_bound(NEW."address_domain_folded", OLD."address_domain_folded", 20000) OR
public.text_value_exceeds_bound(NEW."channel_user_id", OLD."channel_user_id", 20000) OR
public.text_value_exceeds_bound(NEW."display_name", OLD."display_name", 120) OR
public.text_value_exceeds_bound(NEW."domain_evidence_precision", OLD."domain_evidence_precision", 20000) OR
public.text_value_exceeds_bound(NEW."response", OLD."response", 20000) OR
public.text_value_exceeds_bound(NEW."role", OLD."role", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_participant_replay" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."outcome", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_participant_replay" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_reader_state" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."reopen_on", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."set_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_reader_state" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."reopen_on", OLD."reopen_on", 20000) OR
public.text_value_exceeds_bound(NEW."set_by", OLD."set_by", 20000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_reply_verdict_history" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."decided_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."verdict", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_reply_verdict_history" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."decided_by", OLD."decided_by", 20000) OR
public.text_value_exceeds_bound(NEW."verdict", OLD."verdict", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_request_settlement" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."decided_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."judged_evidence", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."remaining", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."rule_version", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."verdict", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_request_settlement" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."decided_by", OLD."decided_by", 20000) OR
public.text_value_exceeds_bound(NEW."judged_evidence", OLD."judged_evidence", 20000) OR
public.text_value_exceeds_bound(NEW."remaining", OLD."remaining", 20000) OR
public.text_value_exceeds_bound(NEW."rule_version", OLD."rule_version", 20000) OR
public.text_value_exceeds_bound(NEW."verdict", OLD."verdict", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_retention_evidence" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."basis", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."deal_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."decided_by_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."project_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."reason", NULL, 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_retention_evidence" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."basis", OLD."basis", 20000) OR
public.text_value_exceeds_bound(NEW."deal_name", OLD."deal_name", 120) OR
public.text_value_exceeds_bound(NEW."decided_by_name", OLD."decided_by_name", 120) OR
public.text_value_exceeds_bound(NEW."project_name", OLD."project_name", 120) OR
public.text_value_exceeds_bound(NEW."reason", OLD."reason", 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_review_response" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."outcome", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."template_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_review_response" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."template_key", OLD."template_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_review_template" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."outcome", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."shipped_label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."shipped_label_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_review_template" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."key", OLD."key", 20000) OR
public.text_value_exceeds_bound(NEW."label", OLD."label", 120) OR
public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000) OR
public.text_value_exceeds_bound(NEW."shipped_label", OLD."shipped_label", 120) OR
public.text_value_exceeds_bound(NEW."shipped_label_key", OLD."shipped_label_key", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."activity_sales_state" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."channel_provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."set_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."thread_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."activity_sales_state" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."channel_provider", OLD."channel_provider", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."set_by", OLD."set_by", 20000) OR
public.text_value_exceeds_bound(NEW."thread_key", OLD."thread_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."agent_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."agent_spec", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."degrade_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."goal", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."trigger_ref", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."agent_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."agent_spec", OLD."agent_spec", 20000) OR
public.text_value_exceeds_bound(NEW."degrade_reason", OLD."degrade_reason", 2000) OR
public.text_value_exceeds_bound(NEW."goal", OLD."goal", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."trigger_ref", OLD."trigger_ref", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."agent_standing_grant" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."agent_spec", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."agent_standing_grant" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."agent_spec", OLD."agent_spec", 20000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."agent_task" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status_message", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."tool", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."agent_task" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."status_message", OLD."status_message", 20000) OR
public.text_value_exceeds_bound(NEW."tool", OLD."tool", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."ai_call" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."attempt_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."config_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."context_fingerprint", NULL, 20000) OR
public.text_values_exceed_bound(NEW."context_scopes", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."decision_choice", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."error_sentinel", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."finish_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."model_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."request_fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."schema_downgrade", NULL, 20000) OR
public.text_values_exceed_bound(NEW."secret_kinds", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."served_identity_source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."served_model", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."served_provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."task", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."tier", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."ai_call" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."attempt_reason", OLD."attempt_reason", 2000) OR
public.text_value_exceeds_bound(NEW."config_hash", OLD."config_hash", 20000) OR
public.text_value_exceeds_bound(NEW."context_fingerprint", OLD."context_fingerprint", 20000) OR
public.text_values_exceed_bound(NEW."context_scopes", OLD."context_scopes", 20000) OR
public.text_value_exceeds_bound(NEW."decision_choice", OLD."decision_choice", 20000) OR
public.text_value_exceeds_bound(NEW."error_sentinel", OLD."error_sentinel", 20000) OR
public.text_value_exceeds_bound(NEW."finish_reason", OLD."finish_reason", 2000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."model_id", OLD."model_id", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_value_exceeds_bound(NEW."request_fingerprint", OLD."request_fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."schema_downgrade", OLD."schema_downgrade", 20000) OR
public.text_values_exceed_bound(NEW."secret_kinds", OLD."secret_kinds", 20000) OR
public.text_value_exceeds_bound(NEW."served_identity_source", OLD."served_identity_source", 20000) OR
public.text_value_exceeds_bound(NEW."served_model", OLD."served_model", 20000) OR
public.text_value_exceeds_bound(NEW."served_provider", OLD."served_provider", 20000) OR
public.text_value_exceeds_bound(NEW."subject_type", OLD."subject_type", 20000) OR
public.text_value_exceeds_bound(NEW."task", OLD."task", 20000) OR
public.text_value_exceeds_bound(NEW."tier", OLD."tier", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."ai_call_config" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."prompt_version", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."routing_config_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."task_contract_hash", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."ai_call_config" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."hash", OLD."hash", 20000) OR
public.text_value_exceeds_bound(NEW."prompt_version", OLD."prompt_version", 20000) OR
public.text_value_exceeds_bound(NEW."routing_config_hash", OLD."routing_config_hash", 20000) OR
public.text_value_exceeds_bound(NEW."task_contract_hash", OLD."task_contract_hash", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."ai_feedback" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."claim_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."claim_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."corrected_value", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."value_shown", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."verdict", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."ai_feedback" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."claim_key", OLD."claim_key", 20000) OR
public.text_value_exceeds_bound(NEW."claim_kind", OLD."claim_kind", 20000) OR
public.text_value_exceeds_bound(NEW."corrected_value", OLD."corrected_value", 20000) OR
public.text_value_exceeds_bound(NEW."note", OLD."note", 2000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."subject_type", OLD."subject_type", 20000) OR
public.text_value_exceeds_bound(NEW."value_shown", OLD."value_shown", 20000) OR
public.text_value_exceeds_bound(NEW."verdict", OLD."verdict", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."ai_model_rate" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."lane", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."model_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."ai_model_rate" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."lane", OLD."lane", 20000) OR
public.text_value_exceeds_bound(NEW."model_id", OLD."model_id", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."ai_task_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."actor_scope", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."ai_task", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."degrade_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."occurrence_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."quantity_unit", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject_label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."subject_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."summary", NULL, 500)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."ai_task_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."actor_scope", OLD."actor_scope", 20000) OR
public.text_value_exceeds_bound(NEW."ai_task", OLD."ai_task", 20000) OR
public.text_value_exceeds_bound(NEW."degrade_reason", OLD."degrade_reason", 2000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."occurrence_key", OLD."occurrence_key", 20000) OR
public.text_value_exceeds_bound(NEW."quantity_unit", OLD."quantity_unit", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000) OR
public.text_value_exceeds_bound(NEW."subject_label", OLD."subject_label", 120) OR
public.text_value_exceeds_bound(NEW."subject_type", OLD."subject_type", 20000) OR
public.text_value_exceeds_bound(NEW."summary", OLD."summary", 500)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."ai_usage" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."task", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."tier", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."ai_usage" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."task", OLD."task", 20000) OR
public.text_value_exceeds_bound(NEW."tier", OLD."tier", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."analytics_share" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."scope_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."target", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."token_hash", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."analytics_share" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."scope_kind", OLD."scope_kind", 20000) OR
public.text_value_exceeds_bound(NEW."target", OLD."target", 20000) OR
public.text_value_exceeds_bound(NEW."token_hash", OLD."token_hash", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."app_user" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."avatar_object_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."avatar_origin", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."display_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."email", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."email_domain_folded", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."greeting_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."locale", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."morning_brief_delivery", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."password_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."routine_delivery_time", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."routine_mail_scope", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."seat_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."service_extension", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."timezone", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."timezone_set_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."weekly_delivery", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."app_user" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."avatar_object_key", OLD."avatar_object_key", 20000) OR
public.text_value_exceeds_bound(NEW."avatar_origin", OLD."avatar_origin", 20000) OR
public.text_value_exceeds_bound(NEW."display_name", OLD."display_name", 120) OR
public.text_value_exceeds_bound(NEW."email", OLD."email", 20000) OR
public.text_value_exceeds_bound(NEW."email_domain_folded", OLD."email_domain_folded", 20000) OR
public.text_value_exceeds_bound(NEW."greeting_name", OLD."greeting_name", 120) OR
public.text_value_exceeds_bound(NEW."locale", OLD."locale", 20000) OR
public.text_value_exceeds_bound(NEW."morning_brief_delivery", OLD."morning_brief_delivery", 20000) OR
public.text_value_exceeds_bound(NEW."password_hash", OLD."password_hash", 20000) OR
public.text_value_exceeds_bound(NEW."routine_delivery_time", OLD."routine_delivery_time", 20000) OR
public.text_value_exceeds_bound(NEW."routine_mail_scope", OLD."routine_mail_scope", 20000) OR
public.text_value_exceeds_bound(NEW."seat_type", OLD."seat_type", 20000) OR
public.text_value_exceeds_bound(NEW."service_extension", OLD."service_extension", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."timezone", OLD."timezone", 20000) OR
public.text_value_exceeds_bound(NEW."timezone_set_by", OLD."timezone_set_by", 20000) OR
public.text_value_exceeds_bound(NEW."weekly_delivery", OLD."weekly_delivery", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."approval" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."co_target_entity_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."context_digest", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."decision_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."diff_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."effect_failure", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."proposed_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."requested_decider_source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."summary", NULL, 500) OR
public.text_value_exceeds_bound(NEW."target_entity_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."target_label", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."approval" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."co_target_entity_type", OLD."co_target_entity_type", 20000) OR
public.text_value_exceeds_bound(NEW."context_digest", OLD."context_digest", 20000) OR
public.text_value_exceeds_bound(NEW."decision_reason", OLD."decision_reason", 2000) OR
public.text_value_exceeds_bound(NEW."diff_hash", OLD."diff_hash", 20000) OR
public.text_value_exceeds_bound(NEW."effect_failure", OLD."effect_failure", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."proposed_by", OLD."proposed_by", 20000) OR
public.text_value_exceeds_bound(NEW."requested_decider_source", OLD."requested_decider_source", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."summary", OLD."summary", 500) OR
public.text_value_exceeds_bound(NEW."target_entity_type", OLD."target_entity_type", 20000) OR
public.text_value_exceeds_bound(NEW."target_label", OLD."target_label", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."approval_autonomy_policy" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."mode", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."approval_autonomy_policy" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."mode", OLD."mode", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."approval_preference" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."approval_preference" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."assurance_cycle" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."opened_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."scope", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."assurance_cycle" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."opened_by", OLD."opened_by", 20000) OR
public.text_value_exceeds_bound(NEW."scope", OLD."scope", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."assurance_deal_receipt" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."input_fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."outcome", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."policy_version", NULL, 20000) OR
public.text_values_exceed_bound(NEW."reasons", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."verdict_basis_digest", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."verdict_input_checksum", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."verdict_prompt_version", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."assurance_deal_receipt" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."input_fingerprint", OLD."input_fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000) OR
public.text_value_exceeds_bound(NEW."policy_version", OLD."policy_version", 20000) OR
public.text_values_exceed_bound(NEW."reasons", OLD."reasons", 2000) OR
public.text_value_exceeds_bound(NEW."verdict_basis_digest", OLD."verdict_basis_digest", 20000) OR
public.text_value_exceeds_bound(NEW."verdict_input_checksum", OLD."verdict_input_checksum", 20000) OR
public.text_value_exceeds_bound(NEW."verdict_prompt_version", OLD."verdict_prompt_version", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."assurance_exception" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."delivery_state", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."logical_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."severity", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."assurance_exception" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."delivery_state", OLD."delivery_state", 20000) OR
public.text_value_exceeds_bound(NEW."logical_key", OLD."logical_key", 20000) OR
public.text_value_exceeds_bound(NEW."severity", OLD."severity", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."subject_kind", OLD."subject_kind", 20000) OR
public.text_value_exceeds_bound(NEW."type", OLD."type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."assurance_input_change" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."subject_kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."assurance_input_change" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."subject_kind", OLD."subject_kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."assurance_resolution" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."evidence_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."outcome", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason", NULL, 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."assurance_resolution" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."evidence_ref", OLD."evidence_ref", 20000) OR
public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000) OR
public.text_value_exceeds_bound(NEW."reason", OLD."reason", 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."assurance_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."digest", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."readiness", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."requested_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."assurance_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."digest", OLD."digest", 20000) OR
public.text_value_exceeds_bound(NEW."readiness", OLD."readiness", 20000) OR
public.text_value_exceeds_bound(NEW."requested_by", OLD."requested_by", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."assurance_source_coverage" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."assurance_source_coverage" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."assurance_task_item" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."state", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject_kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."assurance_task_item" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."state", OLD."state", 20000) OR
public.text_value_exceeds_bound(NEW."subject_kind", OLD."subject_kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."attachment" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."category", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."checksum", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."content_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."declared_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."doc_state", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."external_part_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."external_source_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."filename", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."storage_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."title", NULL, 120) OR
public.text_value_exceeds_bound(NEW."url", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."attachment" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."category", OLD."category", 20000) OR
public.text_value_exceeds_bound(NEW."checksum", OLD."checksum", 20000) OR
public.text_value_exceeds_bound(NEW."content_type", OLD."content_type", 20000) OR
public.text_value_exceeds_bound(NEW."declared_type", OLD."declared_type", 20000) OR
public.text_value_exceeds_bound(NEW."doc_state", OLD."doc_state", 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", OLD."entity_type", 20000) OR
public.text_value_exceeds_bound(NEW."external_part_id", OLD."external_part_id", 20000) OR
public.text_value_exceeds_bound(NEW."external_source_id", OLD."external_source_id", 20000) OR
public.text_value_exceeds_bound(NEW."filename", OLD."filename", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."storage_key", OLD."storage_key", 20000) OR
public.text_value_exceeds_bound(NEW."title", OLD."title", 120) OR
public.text_value_exceeds_bound(NEW."url", OLD."url", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."attachment_extraction" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."question", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."requested_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status_detail", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."attachment_extraction" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."question", OLD."question", 20000) OR
public.text_value_exceeds_bound(NEW."requested_by", OLD."requested_by", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."status_detail", OLD."status_detail", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."audit_log" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."action", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."actor_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."actor_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."authorization_rule", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."audit_log" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."action", OLD."action", 20000) OR
public.text_value_exceeds_bound(NEW."actor_id", OLD."actor_id", 20000) OR
public.text_value_exceeds_bound(NEW."actor_type", OLD."actor_type", 20000) OR
public.text_value_exceeds_bound(NEW."authorization_rule", OLD."authorization_rule", 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", OLD."entity_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."auth_token" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."purpose", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."token_hash", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."auth_token" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."purpose", OLD."purpose", 20000) OR
public.text_value_exceeds_bound(NEW."token_hash", OLD."token_hash", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."automation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."origin", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."paused_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."tier", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."automation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."key", OLD."key", 20000) OR
public.text_value_exceeds_bound(NEW."name", OLD."name", 120) OR
public.text_value_exceeds_bound(NEW."origin", OLD."origin", 20000) OR
public.text_value_exceeds_bound(NEW."paused_reason", OLD."paused_reason", 2000) OR
public.text_value_exceeds_bound(NEW."tier", OLD."tier", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."automation_effect_claim" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."effect_fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."handler", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."occurrence_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."automation_effect_claim" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."effect_fingerprint", OLD."effect_fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."handler", OLD."handler", 20000) OR
public.text_value_exceeds_bound(NEW."occurrence_key", OLD."occurrence_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."booking_alias" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."alias", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."booking_alias" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."alias", OLD."alias", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."booking_page" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."slug", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."booking_page" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."slug", OLD."slug", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."brief_item" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."finding", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reopen_on", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."brief_item" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."finding", OLD."finding", 20000) OR
public.text_value_exceeds_bound(NEW."reopen_on", OLD."reopen_on", 20000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."brief_run" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."factors_omitted", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."mail_error", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."narrative", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."revenue_norm_currency", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."brief_run" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."factors_omitted", OLD."factors_omitted", 20000) OR
public.text_value_exceeds_bound(NEW."mail_error", OLD."mail_error", 20000) OR
public.text_value_exceeds_bound(NEW."narrative", OLD."narrative", 20000) OR
public.text_value_exceeds_bound(NEW."revenue_norm_currency", OLD."revenue_norm_currency", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."bulk_confirmation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."requested_by", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."bulk_confirmation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."requested_by", OLD."requested_by", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."bulk_operation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."record_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."requested_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."verb", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."bulk_operation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."record_type", OLD."record_type", 20000) OR
public.text_value_exceeds_bound(NEW."requested_by", OLD."requested_by", 20000) OR
public.text_value_exceeds_bound(NEW."verb", OLD."verb", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_alias_refused" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."value", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_alias_refused" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."value", OLD."value", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_alias_sighting" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."value", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_alias_sighting" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."value", OLD."value", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_auto_enrich_state" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."last_outcome", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_auto_enrich_state" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."last_outcome", OLD."last_outcome", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_backfill" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."last_error_class", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_backfill" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."last_error_class", OLD."last_error_class", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_backfill_creation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_backfill_creation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."subject", OLD."subject", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_connection" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."account_label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."credential_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."history_baseline_set_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."history_baseline_tz", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."mail_posture", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_values_exceed_bound(NEW."provider_scopes", NULL, 20000) OR
public.text_values_exceed_bound(NEW."scopes", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."watch_ref", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_connection" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."account_label", OLD."account_label", 120) OR
public.text_value_exceeds_bound(NEW."credential_ref", OLD."credential_ref", 20000) OR
public.text_value_exceeds_bound(NEW."history_baseline_set_by", OLD."history_baseline_set_by", 20000) OR
public.text_value_exceeds_bound(NEW."history_baseline_tz", OLD."history_baseline_tz", 20000) OR
public.text_value_exceeds_bound(NEW."mail_posture", OLD."mail_posture", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_values_exceed_bound(NEW."provider_scopes", OLD."provider_scopes", 20000) OR
public.text_values_exceed_bound(NEW."scopes", OLD."scopes", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."watch_ref", OLD."watch_ref", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_counterparty_hold" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."created_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."value", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_counterparty_hold" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."created_by", OLD."created_by", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."value", OLD."value", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_exclusion" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."created_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."scope", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."value", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_exclusion" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."created_by", OLD."created_by", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."scope", OLD."scope", 20000) OR
public.text_value_exceeds_bound(NEW."value", OLD."value", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_freemail_domain" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."domain", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_freemail_domain" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."domain", OLD."domain", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_import" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."delivered_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."posture_at_import", NULL, 20000) OR
public.text_values_exceed_bound(NEW."provider_message_ids", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."verdict_reason", NULL, 2000) OR
public.text_values_exceed_bound(NEW."verdict_reasons", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."verdict_status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_import" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."delivered_by", OLD."delivered_by", 20000) OR
public.text_value_exceeds_bound(NEW."posture_at_import", OLD."posture_at_import", 20000) OR
public.text_values_exceed_bound(NEW."provider_message_ids", OLD."provider_message_ids", 20000) OR
public.text_value_exceeds_bound(NEW."verdict_reason", OLD."verdict_reason", 2000) OR
public.text_values_exceed_bound(NEW."verdict_reasons", OLD."verdict_reasons", 2000) OR
public.text_value_exceeds_bound(NEW."verdict_status", OLD."verdict_status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_owner_identity" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."created_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."value", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_owner_identity" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."created_by", OLD."created_by", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."value", OLD."value", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_owner_identity_withdrawn" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."created_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."value", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."withdrawn_by", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_owner_identity_withdrawn" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."created_by", OLD."created_by", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."value", OLD."value", 20000) OR
public.text_value_exceeds_bound(NEW."withdrawn_by", OLD."withdrawn_by", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_party_source" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."containers", NULL, 20000) OR
public.text_values_exceed_bound(NEW."policy_addresses", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_values_exceed_bound(NEW."recipients", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_party_source" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."containers", OLD."containers", 20000) OR
public.text_values_exceed_bound(NEW."policy_addresses", OLD."policy_addresses", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_values_exceed_bound(NEW."recipients", OLD."recipients", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_party_source_replay" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."outcome", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_party_source_replay" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_pending_counterparty" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."display_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."disposition_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."domain", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."email", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."served_model", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_pending_counterparty" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."display_name", OLD."display_name", 120) OR
public.text_value_exceeds_bound(NEW."disposition_reason", OLD."disposition_reason", 2000) OR
public.text_value_exceeds_bound(NEW."domain", OLD."domain", 20000) OR
public.text_value_exceeds_bound(NEW."email", OLD."email", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."served_model", OLD."served_model", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_sender_override" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."address", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."decision", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."overruled_kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_sender_override" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."address", OLD."address", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."decision", OLD."decision", 20000) OR
public.text_value_exceeds_bound(NEW."overruled_kind", OLD."overruled_kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_sweep_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."error_class", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."outcome", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."sweep", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_sweep_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."error_class", OLD."error_class", 20000) OR
public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000) OR
public.text_value_exceeds_bound(NEW."sweep", OLD."sweep", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_sync_state" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."last_error_class", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_sync_state" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."last_error_class", OLD."last_error_class", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_test_mailbox_sent" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."message_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_test_mailbox_sent" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."message_id", OLD."message_id", 20000) OR
public.text_value_exceeds_bound(NEW."subject", OLD."subject", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_thread_verdict" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."disposition_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_values_exceed_bound(NEW."seen_addresses", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."thread_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_thread_verdict" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."disposition_reason", OLD."disposition_reason", 2000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_values_exceed_bound(NEW."seen_addresses", OLD."seen_addresses", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."thread_key", OLD."thread_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."capture_trace" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."connector", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."counterparty", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."outcome", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."source_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_system", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."stage", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."capture_trace" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."connector", OLD."connector", 20000) OR
public.text_value_exceeds_bound(NEW."counterparty", OLD."counterparty", 20000) OR
public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000) OR
public.text_value_exceeds_bound(NEW."reason", OLD."reason", 2000) OR
public.text_value_exceeds_bound(NEW."source_id", OLD."source_id", 20000) OR
public.text_value_exceeds_bound(NEW."source_system", OLD."source_system", 20000) OR
public.text_value_exceeds_bound(NEW."stage", OLD."stage", 20000) OR
public.text_value_exceeds_bound(NEW."subject", OLD."subject", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."channel_connection" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."channel_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."channel_label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."credential_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."channel_connection" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."channel_id", OLD."channel_id", 20000) OR
public.text_value_exceeds_bound(NEW."channel_label", OLD."channel_label", 120) OR
public.text_value_exceeds_bound(NEW."credential_ref", OLD."credential_ref", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."channel_provider" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."credential_model", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."transport", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."channel_provider" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."credential_model", OLD."credential_model", 20000) OR
public.text_value_exceeds_bound(NEW."label", OLD."label", 120) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_value_exceeds_bound(NEW."transport", OLD."transport", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."close_date_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."close_date_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."close_date_run_member" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."outcome", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."close_date_run_member" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."commission_entry" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."attribution_at_accrual", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."margin_tier_at_accrual", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."void_reason", NULL, 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."commission_entry" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."attribution_at_accrual", OLD."attribution_at_accrual", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."margin_tier_at_accrual", OLD."margin_tier_at_accrual", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."void_reason", OLD."void_reason", 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."comms_outbound" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."body", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."bounce_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."bounce_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."bounce_recipient", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."channel_user_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."consent_purpose", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."execution_authority", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."from_address", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."from_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."html_body", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."in_reply_to", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."inline_logo_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."list_unsubscribe", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."message_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."payload_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider_message_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason", NULL, 2000) OR
public.text_values_exceed_bound(NEW."redacted_fields", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."sender_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject", NULL, 120) OR
public.text_value_exceeds_bound(NEW."template_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."thread_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."comms_outbound" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."body", OLD."body", 20000) OR
public.text_value_exceeds_bound(NEW."bounce_kind", OLD."bounce_kind", 20000) OR
public.text_value_exceeds_bound(NEW."bounce_reason", OLD."bounce_reason", 2000) OR
public.text_value_exceeds_bound(NEW."bounce_recipient", OLD."bounce_recipient", 20000) OR
public.text_value_exceeds_bound(NEW."channel_user_id", OLD."channel_user_id", 20000) OR
public.text_value_exceeds_bound(NEW."consent_purpose", OLD."consent_purpose", 20000) OR
public.text_value_exceeds_bound(NEW."execution_authority", OLD."execution_authority", 20000) OR
public.text_value_exceeds_bound(NEW."from_address", OLD."from_address", 20000) OR
public.text_value_exceeds_bound(NEW."from_name", OLD."from_name", 120) OR
public.text_value_exceeds_bound(NEW."html_body", OLD."html_body", 20000) OR
public.text_value_exceeds_bound(NEW."in_reply_to", OLD."in_reply_to", 20000) OR
public.text_value_exceeds_bound(NEW."inline_logo_key", OLD."inline_logo_key", 20000) OR
public.text_value_exceeds_bound(NEW."list_unsubscribe", OLD."list_unsubscribe", 20000) OR
public.text_value_exceeds_bound(NEW."message_id", OLD."message_id", 20000) OR
public.text_value_exceeds_bound(NEW."payload_ref", OLD."payload_ref", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_value_exceeds_bound(NEW."provider_message_id", OLD."provider_message_id", 20000) OR
public.text_value_exceeds_bound(NEW."reason", OLD."reason", 2000) OR
public.text_values_exceed_bound(NEW."redacted_fields", OLD."redacted_fields", 20000) OR
public.text_value_exceeds_bound(NEW."sender_kind", OLD."sender_kind", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."subject", OLD."subject", 120) OR
public.text_value_exceeds_bound(NEW."template_key", OLD."template_key", 20000) OR
public.text_value_exceeds_bound(NEW."thread_key", OLD."thread_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."communication_basis" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."thread_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."communication_basis" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."note", OLD."note", 2000) OR
public.text_value_exceeds_bound(NEW."thread_key", OLD."thread_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."communication_decision" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."actor", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."basis", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."data_protection_country", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."execution_authority", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."legacy_verdict", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."mode", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."phase", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."purpose_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason_code", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."recipient_address", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."requested_category", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."resolved_category", NULL, 20000) OR
public.text_values_exceed_bound(NEW."ruleset_codes", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."suppression", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."verdict", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."communication_decision" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."actor", OLD."actor", 20000) OR
public.text_value_exceeds_bound(NEW."basis", OLD."basis", 20000) OR
public.text_value_exceeds_bound(NEW."data_protection_country", OLD."data_protection_country", 20000) OR
public.text_value_exceeds_bound(NEW."execution_authority", OLD."execution_authority", 20000) OR
public.text_value_exceeds_bound(NEW."legacy_verdict", OLD."legacy_verdict", 20000) OR
public.text_value_exceeds_bound(NEW."mode", OLD."mode", 20000) OR
public.text_value_exceeds_bound(NEW."phase", OLD."phase", 20000) OR
public.text_value_exceeds_bound(NEW."purpose_key", OLD."purpose_key", 20000) OR
public.text_value_exceeds_bound(NEW."reason_code", OLD."reason_code", 2000) OR
public.text_value_exceeds_bound(NEW."recipient_address", OLD."recipient_address", 20000) OR
public.text_value_exceeds_bound(NEW."requested_category", OLD."requested_category", 20000) OR
public.text_value_exceeds_bound(NEW."resolved_category", OLD."resolved_category", 20000) OR
public.text_values_exceed_bound(NEW."ruleset_codes", OLD."ruleset_codes", 20000) OR
public.text_value_exceeds_bound(NEW."subject_kind", OLD."subject_kind", 20000) OR
public.text_value_exceeds_bound(NEW."suppression", OLD."suppression", 20000) OR
public.text_value_exceeds_bound(NEW."verdict", OLD."verdict", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."communication_instruction" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."directed_by_agent", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."explanation", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason_code", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."revoked_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."warning_version", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."communication_instruction" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."directed_by_agent", OLD."directed_by_agent", 20000) OR
public.text_value_exceeds_bound(NEW."explanation", OLD."explanation", 20000) OR
public.text_value_exceeds_bound(NEW."reason_code", OLD."reason_code", 2000) OR
public.text_value_exceeds_bound(NEW."revoked_reason", OLD."revoked_reason", 2000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."warning_version", OLD."warning_version", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."communication_override" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."category", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."decided_by_level", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason", NULL, 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."communication_override" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."category", OLD."category", 20000) OR
public.text_value_exceeds_bound(NEW."decided_by_level", OLD."decided_by_level", 20000) OR
public.text_value_exceeds_bound(NEW."reason", OLD."reason", 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."communication_review" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason_code", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."communication_review" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."reason_code", OLD."reason_code", 2000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."communication_suppression" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."address", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."address_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."address_key_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."decided_by_level", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_values_exceed_bound(NEW."recorded_for_addresses", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."recorded_through", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."communication_suppression" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."address", OLD."address", 20000) OR
public.text_value_exceeds_bound(NEW."address_hash", OLD."address_hash", 20000) OR
public.text_value_exceeds_bound(NEW."address_key_id", OLD."address_key_id", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."decided_by_level", OLD."decided_by_level", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_values_exceed_bound(NEW."recorded_for_addresses", OLD."recorded_for_addresses", 20000) OR
public.text_value_exceeds_bound(NEW."recorded_through", OLD."recorded_through", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."company" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."address_city", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."address_country", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."address_line1", NULL, 500) OR
public.text_value_exceeds_bound(NEW."address_line2", NULL, 500) OR
public.text_value_exceeds_bound(NEW."address_postal_code", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."address_region", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."description", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."display_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."email", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."geocode_input_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."geocode_provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."geocode_status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."industry", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."legal_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."lifecycle", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."linkedin_url", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."logo_icon_object_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."logo_icon_origin", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."logo_object_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."logo_origin", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."name_source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."phone", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."size_band", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_author_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."source_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_system", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."visibility", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."company" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."address_city", OLD."address_city", 20000) OR
public.text_value_exceeds_bound(NEW."address_country", OLD."address_country", 20000) OR
public.text_value_exceeds_bound(NEW."address_line1", OLD."address_line1", 500) OR
public.text_value_exceeds_bound(NEW."address_line2", OLD."address_line2", 500) OR
public.text_value_exceeds_bound(NEW."address_postal_code", OLD."address_postal_code", 20000) OR
public.text_value_exceeds_bound(NEW."address_region", OLD."address_region", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."description", OLD."description", 20000) OR
public.text_value_exceeds_bound(NEW."display_name", OLD."display_name", 120) OR
public.text_value_exceeds_bound(NEW."email", OLD."email", 20000) OR
public.text_value_exceeds_bound(NEW."geocode_input_hash", OLD."geocode_input_hash", 20000) OR
public.text_value_exceeds_bound(NEW."geocode_provider", OLD."geocode_provider", 20000) OR
public.text_value_exceeds_bound(NEW."geocode_status", OLD."geocode_status", 20000) OR
public.text_value_exceeds_bound(NEW."industry", OLD."industry", 20000) OR
public.text_value_exceeds_bound(NEW."legal_name", OLD."legal_name", 120) OR
public.text_value_exceeds_bound(NEW."lifecycle", OLD."lifecycle", 20000) OR
public.text_value_exceeds_bound(NEW."linkedin_url", OLD."linkedin_url", 20000) OR
public.text_value_exceeds_bound(NEW."logo_icon_object_key", OLD."logo_icon_object_key", 20000) OR
public.text_value_exceeds_bound(NEW."logo_icon_origin", OLD."logo_icon_origin", 20000) OR
public.text_value_exceeds_bound(NEW."logo_object_key", OLD."logo_object_key", 20000) OR
public.text_value_exceeds_bound(NEW."logo_origin", OLD."logo_origin", 20000) OR
public.text_value_exceeds_bound(NEW."name_source", OLD."name_source", 20000) OR
public.text_value_exceeds_bound(NEW."phone", OLD."phone", 20000) OR
public.text_value_exceeds_bound(NEW."size_band", OLD."size_band", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."source_author_name", OLD."source_author_name", 120) OR
public.text_value_exceeds_bound(NEW."source_id", OLD."source_id", 20000) OR
public.text_value_exceeds_bound(NEW."source_system", OLD."source_system", 20000) OR
public.text_value_exceeds_bound(NEW."visibility", OLD."visibility", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."company_address_country" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."countries", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."company_address_country" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."countries", OLD."countries", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."company_brief" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."generated_by", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."company_brief" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."fingerprint", OLD."fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."generated_by", OLD."generated_by", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."company_domain" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."domain", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."domain_folded", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."company_domain" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."domain", OLD."domain", 20000) OR
public.text_value_exceeds_bound(NEW."domain_folded", OLD."domain_folded", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."company_domain_disposition" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."admission", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."admission_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."admission_reason_code", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."admission_sender_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."admission_source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."domain", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."evidence", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."pending_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."company_domain_disposition" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."admission", OLD."admission", 20000) OR
public.text_value_exceeds_bound(NEW."admission_reason", OLD."admission_reason", 2000) OR
public.text_value_exceeds_bound(NEW."admission_reason_code", OLD."admission_reason_code", 2000) OR
public.text_value_exceeds_bound(NEW."admission_sender_kind", OLD."admission_sender_kind", 20000) OR
public.text_value_exceeds_bound(NEW."admission_source", OLD."admission_source", 20000) OR
public.text_value_exceeds_bound(NEW."domain", OLD."domain", 20000) OR
public.text_value_exceeds_bound(NEW."evidence", OLD."evidence", 20000) OR
public.text_value_exceeds_bound(NEW."pending_reason", OLD."pending_reason", 2000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."company_domain_question_reader" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."domain", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."company_domain_question_reader" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."domain", OLD."domain", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."company_dossier" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."generated_by", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."company_dossier" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."fingerprint", OLD."fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."generated_by", OLD."generated_by", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."company_fact" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."category", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."evidence_snippet", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."field", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_url", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."value", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."value_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."company_fact" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."category", OLD."category", 20000) OR
public.text_value_exceeds_bound(NEW."evidence_snippet", OLD."evidence_snippet", 20000) OR
public.text_value_exceeds_bound(NEW."field", OLD."field", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."source_url", OLD."source_url", 20000) OR
public.text_value_exceeds_bound(NEW."value", OLD."value", 20000) OR
public.text_value_exceeds_bound(NEW."value_key", OLD."value_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."company_geocode_state" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."input_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."last_outcome", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."company_geocode_state" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."input_hash", OLD."input_hash", 20000) OR
public.text_value_exceeds_bound(NEW."last_outcome", OLD."last_outcome", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."company_growth_fit" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."generated_by", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."company_growth_fit" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."fingerprint", OLD."fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."generated_by", OLD."generated_by", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."company_offered_value" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."field_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."offered_value", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_system", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."company_offered_value" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."field_key", OLD."field_key", 20000) OR
public.text_value_exceeds_bound(NEW."offered_value", OLD."offered_value", 20000) OR
public.text_value_exceeds_bound(NEW."source_system", OLD."source_system", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."company_profile_field" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."evidence_snippet", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."field", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_url", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."value", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."company_profile_field" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."evidence_snippet", OLD."evidence_snippet", 20000) OR
public.text_value_exceeds_bound(NEW."field", OLD."field", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."source_url", OLD."source_url", 20000) OR
public.text_value_exceeds_bound(NEW."value", OLD."value", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."company_relationship_type" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."relationship_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."company_relationship_type" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."relationship_type", OLD."relationship_type", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."company_scan" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."degrade_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."generated_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."company_scan" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."degrade_reason", OLD."degrade_reason", 2000) OR
public.text_value_exceeds_bound(NEW."fingerprint", OLD."fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."generated_by", OLD."generated_by", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."company_technical_state" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."lane", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."last_outcome", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."company_technical_state" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."lane", OLD."lane", 20000) OR
public.text_value_exceeds_bound(NEW."last_outcome", OLD."last_outcome", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."company_vat_check" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."consultation_number", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."registered_address", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."registered_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."vat_number", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."company_vat_check" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."consultation_number", OLD."consultation_number", 20000) OR
public.text_value_exceeds_bound(NEW."registered_address", OLD."registered_address", 20000) OR
public.text_value_exceeds_bound(NEW."registered_name", OLD."registered_name", 120) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."vat_number", OLD."vat_number", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."confirm_token" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."delivered_to", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."question_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."question_locale", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."token_hash", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."confirm_token" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."delivered_to", OLD."delivered_to", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."question_key", OLD."question_key", 20000) OR
public.text_value_exceeds_bound(NEW."question_locale", OLD."question_locale", 20000) OR
public.text_value_exceeds_bound(NEW."token_hash", OLD."token_hash", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."consent_doi_token" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."token_hash", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."consent_doi_token" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."token_hash", OLD."token_hash", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."consent_event" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."confirm_ip", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."confirm_user_agent", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."issuance_trigger", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."lawful_basis", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."new_state", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."policy_text", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."policy_version", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."consent_event" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."confirm_ip", OLD."confirm_ip", 20000) OR
public.text_value_exceeds_bound(NEW."confirm_user_agent", OLD."confirm_user_agent", 20000) OR
public.text_value_exceeds_bound(NEW."issuance_trigger", OLD."issuance_trigger", 20000) OR
public.text_value_exceeds_bound(NEW."lawful_basis", OLD."lawful_basis", 20000) OR
public.text_value_exceeds_bound(NEW."new_state", OLD."new_state", 20000) OR
public.text_value_exceeds_bound(NEW."policy_text", OLD."policy_text", 20000) OR
public.text_value_exceeds_bound(NEW."policy_version", OLD."policy_version", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."consent_purpose" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."class", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."label", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."consent_purpose" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."class", OLD."class", 20000) OR
public.text_value_exceeds_bound(NEW."key", OLD."key", 20000) OR
public.text_value_exceeds_bound(NEW."label", OLD."label", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."consent_qualifying_event" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."note", NULL, 2000) OR
public.text_values_exceed_bound(NEW."recorded_for_addresses", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."retracted_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_entity_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."consent_qualifying_event" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."note", OLD."note", 2000) OR
public.text_values_exceed_bound(NEW."recorded_for_addresses", OLD."recorded_for_addresses", 20000) OR
public.text_value_exceeds_bound(NEW."retracted_by", OLD."retracted_by", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."source_entity_type", OLD."source_entity_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."consent_text_version" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."body", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."channel", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."content_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."jurisdiction", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."locale", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject_line", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."version", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."consent_text_version" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."body", OLD."body", 20000) OR
public.text_value_exceeds_bound(NEW."channel", OLD."channel", 20000) OR
public.text_value_exceeds_bound(NEW."content_hash", OLD."content_hash", 20000) OR
public.text_value_exceeds_bound(NEW."jurisdiction", OLD."jurisdiction", 20000) OR
public.text_value_exceeds_bound(NEW."key", OLD."key", 20000) OR
public.text_value_exceeds_bound(NEW."locale", OLD."locale", 20000) OR
public.text_value_exceeds_bound(NEW."subject_line", OLD."subject_line", 20000) OR
public.text_value_exceeds_bound(NEW."version", OLD."version", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."contact" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."address_city", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."address_country", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."address_line1", NULL, 500) OR
public.text_value_exceeds_bound(NEW."address_line2", NULL, 500) OR
public.text_value_exceeds_bound(NEW."address_postal_code", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."address_region", NULL, 20000) OR
public.text_values_exceed_bound(NEW."anonymized_address_hashes", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."anonymized_address_key_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."first_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."full_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."last_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."narrowing_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."photo_object_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."photo_origin", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."relationship_mark", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."relationship_mark_set_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_author_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."source_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_system", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."title", NULL, 120) OR
public.text_value_exceeds_bound(NEW."visibility", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."contact" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."address_city", OLD."address_city", 20000) OR
public.text_value_exceeds_bound(NEW."address_country", OLD."address_country", 20000) OR
public.text_value_exceeds_bound(NEW."address_line1", OLD."address_line1", 500) OR
public.text_value_exceeds_bound(NEW."address_line2", OLD."address_line2", 500) OR
public.text_value_exceeds_bound(NEW."address_postal_code", OLD."address_postal_code", 20000) OR
public.text_value_exceeds_bound(NEW."address_region", OLD."address_region", 20000) OR
public.text_values_exceed_bound(NEW."anonymized_address_hashes", OLD."anonymized_address_hashes", 20000) OR
public.text_value_exceeds_bound(NEW."anonymized_address_key_id", OLD."anonymized_address_key_id", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."first_name", OLD."first_name", 120) OR
public.text_value_exceeds_bound(NEW."full_name", OLD."full_name", 120) OR
public.text_value_exceeds_bound(NEW."last_name", OLD."last_name", 120) OR
public.text_value_exceeds_bound(NEW."narrowing_reason", OLD."narrowing_reason", 2000) OR
public.text_value_exceeds_bound(NEW."photo_object_key", OLD."photo_object_key", 20000) OR
public.text_value_exceeds_bound(NEW."photo_origin", OLD."photo_origin", 20000) OR
public.text_value_exceeds_bound(NEW."relationship_mark", OLD."relationship_mark", 20000) OR
public.text_value_exceeds_bound(NEW."relationship_mark_set_by", OLD."relationship_mark_set_by", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."source_author_name", OLD."source_author_name", 120) OR
public.text_value_exceeds_bound(NEW."source_id", OLD."source_id", 20000) OR
public.text_value_exceeds_bound(NEW."source_system", OLD."source_system", 20000) OR
public.text_value_exceeds_bound(NEW."title", OLD."title", 120) OR
public.text_value_exceeds_bound(NEW."visibility", OLD."visibility", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."contact_acquisition_evidence" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."purpose_claimed", NULL, 20000) OR
public.text_values_exceed_bound(NEW."recorded_for_addresses", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."request_retracted_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_entity_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."contact_acquisition_evidence" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."note", OLD."note", 2000) OR
public.text_value_exceeds_bound(NEW."purpose_claimed", OLD."purpose_claimed", 20000) OR
public.text_values_exceed_bound(NEW."recorded_for_addresses", OLD."recorded_for_addresses", 20000) OR
public.text_value_exceeds_bound(NEW."request_retracted_by", OLD."request_retracted_by", 20000) OR
public.text_value_exceeds_bound(NEW."source_entity_type", OLD."source_entity_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."contact_address_country" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."countries", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."contact_address_country" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."countries", OLD."countries", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."contact_brief" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."generated_by", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."contact_brief" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."fingerprint", OLD."fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."generated_by", OLD."generated_by", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."contact_channel_identity" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."channel_user_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."membership_bot_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."username", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."contact_channel_identity" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."channel_user_id", OLD."channel_user_id", 20000) OR
public.text_value_exceeds_bound(NEW."membership_bot_id", OLD."membership_bot_id", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."username", OLD."username", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."contact_confirm_submission" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."field", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."proposed_value", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."resolution", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."resolved_by", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."contact_confirm_submission" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."field", OLD."field", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."note", OLD."note", 2000) OR
public.text_value_exceeds_bound(NEW."proposed_value", OLD."proposed_value", 20000) OR
public.text_value_exceeds_bound(NEW."resolution", OLD."resolution", 20000) OR
public.text_value_exceeds_bound(NEW."resolved_by", OLD."resolved_by", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."contact_consent" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."lawful_basis", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."policy_version", NULL, 20000) OR
public.text_values_exceed_bound(NEW."recorded_for_addresses", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."contact_consent" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."lawful_basis", OLD."lawful_basis", 20000) OR
public.text_value_exceeds_bound(NEW."policy_version", OLD."policy_version", 20000) OR
public.text_values_exceed_bound(NEW."recorded_for_addresses", OLD."recorded_for_addresses", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."contact_email" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."email", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."email_domain_folded", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."email_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."contact_email" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."email", OLD."email", 20000) OR
public.text_value_exceeds_bound(NEW."email_domain_folded", OLD."email_domain_folded", 20000) OR
public.text_value_exceeds_bound(NEW."email_type", OLD."email_type", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."contact_moment_dismissal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."claim_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."evidence_fingerprint", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."contact_moment_dismissal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."claim_key", OLD."claim_key", 20000) OR
public.text_value_exceeds_bound(NEW."evidence_fingerprint", OLD."evidence_fingerprint", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."contact_phone" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."phone", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."phone_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."contact_phone" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."phone", OLD."phone", 20000) OR
public.text_value_exceeds_bound(NEW."phone_type", OLD."phone_type", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."contact_profile_field" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."evidence_snippet", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."field", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."superseded_captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."superseded_value", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."value", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."value_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."contact_profile_field" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."evidence_snippet", OLD."evidence_snippet", 20000) OR
public.text_value_exceeds_bound(NEW."field", OLD."field", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."source_ref", OLD."source_ref", 20000) OR
public.text_value_exceeds_bound(NEW."superseded_captured_by", OLD."superseded_captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."superseded_value", OLD."superseded_value", 20000) OR
public.text_value_exceeds_bound(NEW."value", OLD."value", 20000) OR
public.text_value_exceeds_bound(NEW."value_key", OLD."value_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."contact_provider_claim" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."claim_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."employment_processing_error", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."validation_status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."contact_provider_claim" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."claim_key", OLD."claim_key", 20000) OR
public.text_value_exceeds_bound(NEW."employment_processing_error", OLD."employment_processing_error", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."validation_status", OLD."validation_status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."contact_social" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."handle", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."platform", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."contact_social" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."handle", OLD."handle", 20000) OR
public.text_value_exceeds_bound(NEW."platform", OLD."platform", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."contract" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."contract_number", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."title", NULL, 120) OR
public.text_value_exceeds_bound(NEW."value_basis", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."contract" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."contract_number", OLD."contract_number", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."title", OLD."title", 120) OR
public.text_value_exceeds_bound(NEW."value_basis", OLD."value_basis", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."conversation_claim" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."body", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."evidence_fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_quote", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."conversation_claim" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."body", OLD."body", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."evidence_fingerprint", OLD."evidence_fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."source_quote", OLD."source_quote", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."custom_field" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."activity_kinds", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."column_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."object", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."slug", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."custom_field" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."activity_kinds", OLD."activity_kinds", 20000) OR
public.text_value_exceeds_bound(NEW."column_name", OLD."column_name", 120) OR
public.text_value_exceeds_bound(NEW."label", OLD."label", 120) OR
public.text_value_exceeds_bound(NEW."object", OLD."object", 20000) OR
public.text_value_exceeds_bound(NEW."slug", OLD."slug", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."type", OLD."type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."data_subject_request" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."channel", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."due_rule", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."receipt_reference", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."resolution", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject_ref", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."data_subject_request" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."channel", OLD."channel", 20000) OR
public.text_value_exceeds_bound(NEW."due_rule", OLD."due_rule", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."receipt_reference", OLD."receipt_reference", 20000) OR
public.text_value_exceeds_bound(NEW."resolution", OLD."resolution", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."subject_ref", OLD."subject_ref", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."data_task_review" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."operator", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."task_id", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."data_task_review" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."operator", OLD."operator", 20000) OR
public.text_value_exceeds_bound(NEW."task_id", OLD."task_id", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."data_task_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."first_release", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."last_error_class", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."step", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."task_id", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."data_task_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."first_release", OLD."first_release", 20000) OR
public.text_value_exceeds_bound(NEW."last_error_class", OLD."last_error_class", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."step", OLD."step", 20000) OR
public.text_value_exceeds_bound(NEW."task_id", OLD."task_id", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."data_task_run_history" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."step", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."task_id", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."data_task_run_history" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."step", OLD."step", 20000) OR
public.text_value_exceeds_bound(NEW."task_id", OLD."task_id", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."acquisition_source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."commercial_mode", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."commercial_motion", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."description", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."lost_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."lost_reason_basis", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."lost_reason_code", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."partner_attribution", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."priority", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_author_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."source_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_system", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."win_probability_set_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."won_without_contract_detail", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."won_without_contract_reason", NULL, 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."acquisition_source", OLD."acquisition_source", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."commercial_mode", OLD."commercial_mode", 20000) OR
public.text_value_exceeds_bound(NEW."commercial_motion", OLD."commercial_motion", 20000) OR
public.text_value_exceeds_bound(NEW."description", OLD."description", 20000) OR
public.text_value_exceeds_bound(NEW."lost_reason", OLD."lost_reason", 2000) OR
public.text_value_exceeds_bound(NEW."lost_reason_basis", OLD."lost_reason_basis", 2000) OR
public.text_value_exceeds_bound(NEW."lost_reason_code", OLD."lost_reason_code", 2000) OR
public.text_value_exceeds_bound(NEW."name", OLD."name", 120) OR
public.text_value_exceeds_bound(NEW."partner_attribution", OLD."partner_attribution", 20000) OR
public.text_value_exceeds_bound(NEW."priority", OLD."priority", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."source_author_name", OLD."source_author_name", 120) OR
public.text_value_exceeds_bound(NEW."source_id", OLD."source_id", 20000) OR
public.text_value_exceeds_bound(NEW."source_system", OLD."source_system", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."win_probability_set_by", OLD."win_probability_set_by", 20000) OR
public.text_value_exceeds_bound(NEW."won_without_contract_detail", OLD."won_without_contract_detail", 20000) OR
public.text_value_exceeds_bound(NEW."won_without_contract_reason", OLD."won_without_contract_reason", 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_acquisition_source" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."shipped_label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."shipped_label_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_acquisition_source" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."key", OLD."key", 20000) OR
public.text_value_exceeds_bound(NEW."label", OLD."label", 120) OR
public.text_value_exceeds_bound(NEW."shipped_label", OLD."shipped_label", 120) OR
public.text_value_exceeds_bound(NEW."shipped_label_key", OLD."shipped_label_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_correction" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."asking", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."correction", NULL, 20000) OR
public.text_values_exceed_bound(NEW."fields", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."remaining_open_stages", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reversed_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."standing_close_date", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_correction" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."asking", OLD."asking", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."correction", OLD."correction", 20000) OR
public.text_values_exceed_bound(NEW."fields", OLD."fields", 20000) OR
public.text_value_exceeds_bound(NEW."remaining_open_stages", OLD."remaining_open_stages", 20000) OR
public.text_value_exceeds_bound(NEW."reversed_by", OLD."reversed_by", 20000) OR
public.text_value_exceeds_bound(NEW."standing_close_date", OLD."standing_close_date", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_document_hide" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."hidden_by", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_document_hide" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."hidden_by", OLD."hidden_by", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_line_item" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."billing_model", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."catalog_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."component_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."description", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."sku", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."tax_category", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."unit", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_line_item" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."billing_model", OLD."billing_model", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."catalog_name", OLD."catalog_name", 120) OR
public.text_value_exceeds_bound(NEW."component_kind", OLD."component_kind", 20000) OR
public.text_value_exceeds_bound(NEW."description", OLD."description", 20000) OR
public.text_value_exceeds_bound(NEW."sku", OLD."sku", 20000) OR
public.text_value_exceeds_bound(NEW."tax_category", OLD."tax_category", 20000) OR
public.text_value_exceeds_bound(NEW."unit", OLD."unit", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_loss_reason" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."shipped_label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."shipped_label_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_loss_reason" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."key", OLD."key", 20000) OR
public.text_value_exceeds_bound(NEW."label", OLD."label", 120) OR
public.text_value_exceeds_bound(NEW."shipped_label", OLD."shipped_label", 120) OR
public.text_value_exceeds_bound(NEW."shipped_label_key", OLD."shipped_label_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_loss_reason_mapping" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."decided_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."note_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason_key", NULL, 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_loss_reason_mapping" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."decided_by", OLD."decided_by", 20000) OR
public.text_value_exceeds_bound(NEW."note_key", OLD."note_key", 20000) OR
public.text_value_exceeds_bound(NEW."reason_key", OLD."reason_key", 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_move_dismissal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."fingerprint", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_move_dismissal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."fingerprint", OLD."fingerprint", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_product" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."component_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_product" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."component_kind", OLD."component_kind", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_risk_day" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_risk_day" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_room" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."title", NULL, 120) OR
public.text_value_exceeds_bound(NEW."welcome_message", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_room" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000) OR
public.text_value_exceeds_bound(NEW."title", OLD."title", 120) OR
public.text_value_exceeds_bound(NEW."welcome_message", OLD."welcome_message", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_room_comment" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."body", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_room_comment" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."body", OLD."body", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_room_document" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."group_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."title", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_room_document" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."group_key", OLD."group_key", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."title", OLD."title", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_room_engagement" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_room_engagement" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_room_invitation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."failure_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_room_invitation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."failure_reason", OLD."failure_reason", 2000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_room_participant" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."capability", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."email", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."full_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_room_participant" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."capability", OLD."capability", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."email", OLD."email", 20000) OR
public.text_value_exceeds_bound(NEW."full_name", OLD."full_name", 120) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_room_session" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_room_session" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_room_thread" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_room_thread" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_stage_evidence" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."author_side", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."commitment", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."extracted_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."snippet", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_stage_evidence" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."author_side", OLD."author_side", 20000) OR
public.text_value_exceeds_bound(NEW."commitment", OLD."commitment", 20000) OR
public.text_value_exceeds_bound(NEW."extracted_by", OLD."extracted_by", 20000) OR
public.text_value_exceeds_bound(NEW."snippet", OLD."snippet", 20000) OR
public.text_value_exceeds_bound(NEW."source_type", OLD."source_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_stage_history" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."changed_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."lost_reason_code_at_change", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."provenance", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."semantic_at_change", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."valuation_provenance", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_stage_history" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."changed_by", OLD."changed_by", 20000) OR
public.text_value_exceeds_bound(NEW."lost_reason_code_at_change", OLD."lost_reason_code_at_change", 2000) OR
public.text_value_exceeds_bound(NEW."provenance", OLD."provenance", 20000) OR
public.text_value_exceeds_bound(NEW."semantic_at_change", OLD."semantic_at_change", 20000) OR
public.text_value_exceeds_bound(NEW."valuation_provenance", OLD."valuation_provenance", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_status_card" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."generated_by", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_status_card" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."fingerprint", OLD."fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."generated_by", OLD."generated_by", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_suggestion" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."name_hint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_suggestion" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."fingerprint", OLD."fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."name_hint", OLD."name_hint", 20000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."deal_suggestion_evidence" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."deal_suggestion_evidence" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."dedupe_candidate" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."disposition", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."dedupe_candidate" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."disposition", OLD."disposition", 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", OLD."entity_type", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."dedupe_scan_state" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."stream", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."dedupe_scan_state" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."stream", OLD."stream", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."email_signature" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."body", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."html", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."phone", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."title", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."email_signature" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."body", OLD."body", 20000) OR
public.text_value_exceeds_bound(NEW."html", OLD."html", 20000) OR
public.text_value_exceeds_bound(NEW."phone", OLD."phone", 20000) OR
public.text_value_exceeds_bound(NEW."title", OLD."title", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."embed_store_binding" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."populated_identity", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reembedding_identity", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."embed_store_binding" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."populated_identity", OLD."populated_identity", 20000) OR
public.text_value_exceeds_bound(NEW."reembedding_identity", OLD."reembedding_identity", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."embedding" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."chunk_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."model", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."embedding" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."chunk_hash", OLD."chunk_hash", 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", OLD."entity_type", 20000) OR
public.text_value_exceeds_bound(NEW."model", OLD."model", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."erasure_suppression" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."key_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."value_hash", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."erasure_suppression" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."key_id", OLD."key_id", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."value_hash", OLD."value_hash", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."event_outbox" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."stream", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."event_outbox" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."stream", OLD."stream", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."existing_customer_notice" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."collection_channel", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."customer_group", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."evidence_reference", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."notice_text", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."withdrawn_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."withdrawn_reason", NULL, 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."existing_customer_notice" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."collection_channel", OLD."collection_channel", 20000) OR
public.text_value_exceeds_bound(NEW."customer_group", OLD."customer_group", 20000) OR
public.text_value_exceeds_bound(NEW."evidence_reference", OLD."evidence_reference", 20000) OR
public.text_value_exceeds_bound(NEW."notice_text", OLD."notice_text", 20000) OR
public.text_value_exceeds_bound(NEW."withdrawn_by", OLD."withdrawn_by", 20000) OR
public.text_value_exceeds_bound(NEW."withdrawn_reason", OLD."withdrawn_reason", 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."extension_ingest_position" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."position", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_system", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."unit", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."extension_ingest_position" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."position", OLD."position", 20000) OR
public.text_value_exceeds_bound(NEW."source_system", OLD."source_system", 20000) OR
public.text_value_exceeds_bound(NEW."unit", OLD."unit", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."extension_ingest_refusal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."refusal", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."unit", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."extension_ingest_refusal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."refusal", OLD."refusal", 20000) OR
public.text_value_exceeds_bound(NEW."unit", OLD."unit", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."extension_secret" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."extension_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."vault_ref", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."extension_secret" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."extension_name", OLD."extension_name", 120) OR
public.text_value_exceeds_bound(NEW."key", OLD."key", 20000) OR
public.text_value_exceeds_bound(NEW."vault_ref", OLD."vault_ref", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."federated_identity" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."email_at_link", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."federated_identity" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."email_at_link", OLD."email_at_link", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_value_exceeds_bound(NEW."subject", OLD."subject", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."field_mask" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."condition", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."field", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."object", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."role_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."field_mask" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."condition", OLD."condition", 20000) OR
public.text_value_exceeds_bound(NEW."field", OLD."field", 20000) OR
public.text_value_exceeds_bound(NEW."object", OLD."object", 20000) OR
public.text_value_exceeds_bound(NEW."role_key", OLD."role_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."field_provenance" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."evidence_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."field_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."object_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."field_provenance" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."evidence_ref", OLD."evidence_ref", 20000) OR
public.text_value_exceeds_bound(NEW."field_name", OLD."field_name", 120) OR
public.text_value_exceeds_bound(NEW."object_type", OLD."object_type", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."finance_connection" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."credential_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."last_error_code", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."sync_cursor", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."finance_connection" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."credential_ref", OLD."credential_ref", 20000) OR
public.text_value_exceeds_bound(NEW."last_error_code", OLD."last_error_code", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."sync_cursor", OLD."sync_cursor", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."finance_customer_link" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."external_customer_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."sync_hash", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."finance_customer_link" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."external_customer_id", OLD."external_customer_id", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."sync_hash", OLD."sync_hash", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."finance_external_customer" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."display_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."external_customer_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."sync_hash", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."finance_external_customer" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."display_name", OLD."display_name", 120) OR
public.text_value_exceeds_bound(NEW."external_customer_id", OLD."external_customer_id", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."sync_hash", OLD."sync_hash", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."finance_invoice" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."external_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."number", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."raw_status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."sync_hash", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."finance_invoice" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."external_id", OLD."external_id", 20000) OR
public.text_value_exceeds_bound(NEW."number", OLD."number", 20000) OR
public.text_value_exceeds_bound(NEW."raw_status", OLD."raw_status", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."sync_hash", OLD."sync_hash", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."finance_payment" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."external_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."sync_hash", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."finance_payment" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."external_id", OLD."external_id", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."sync_hash", OLD."sync_hash", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."forecast_call" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."scope_kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."forecast_call" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."note", OLD."note", 2000) OR
public.text_value_exceeds_bound(NEW."scope_kind", OLD."scope_kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."forecast_capture_status" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."context_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."failure", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."forecast_capture_status" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."context_key", OLD."context_key", 20000) OR
public.text_value_exceeds_bound(NEW."failure", OLD."failure", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."forecast_contribution" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."category", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."exclusion_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."probability_source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."qualification", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."qualified_policy_version", NULL, 20000) OR
public.text_values_exceed_bound(NEW."risk_signals", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."viability_disposition", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."forecast_contribution" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."category", OLD."category", 20000) OR
public.text_value_exceeds_bound(NEW."exclusion_reason", OLD."exclusion_reason", 2000) OR
public.text_value_exceeds_bound(NEW."probability_source", OLD."probability_source", 20000) OR
public.text_value_exceeds_bound(NEW."qualification", OLD."qualification", 20000) OR
public.text_value_exceeds_bound(NEW."qualified_policy_version", OLD."qualified_policy_version", 20000) OR
public.text_values_exceed_bound(NEW."risk_signals", OLD."risk_signals", 20000) OR
public.text_value_exceeds_bound(NEW."viability_disposition", OLD."viability_disposition", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."forecast_snapshot" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."definition_version", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."forward_measure", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."input_fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."landing_measure", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."population_fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."qualified_policy_version", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."range_quantiles", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."range_version", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."risk_signals_version", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."scope_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."trigger", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."forecast_snapshot" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."definition_version", OLD."definition_version", 20000) OR
public.text_value_exceeds_bound(NEW."forward_measure", OLD."forward_measure", 20000) OR
public.text_value_exceeds_bound(NEW."input_fingerprint", OLD."input_fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."landing_measure", OLD."landing_measure", 20000) OR
public.text_value_exceeds_bound(NEW."population_fingerprint", OLD."population_fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."qualified_policy_version", OLD."qualified_policy_version", 20000) OR
public.text_value_exceeds_bound(NEW."range_quantiles", OLD."range_quantiles", 20000) OR
public.text_value_exceeds_bound(NEW."range_version", OLD."range_version", 20000) OR
public.text_value_exceeds_bound(NEW."risk_signals_version", OLD."risk_signals_version", 20000) OR
public.text_value_exceeds_bound(NEW."scope_kind", OLD."scope_kind", 20000) OR
public.text_value_exceeds_bound(NEW."trigger", OLD."trigger", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."geocode_cache" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."query", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."geocode_cache" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_value_exceeds_bound(NEW."query", OLD."query", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."geocode_country_cache" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."country_codes", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."query", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."geocode_country_cache" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."country_codes", OLD."country_codes", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_value_exceeds_bound(NEW."query", OLD."query", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."home_read_cache" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."read", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."request_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."home_read_cache" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."read", OLD."read", 20000) OR
public.text_value_exceeds_bound(NEW."request_key", OLD."request_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."home_read_fence_pending" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."fence", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."home_read_fence_pending" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."fence", OLD."fence", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."idempotency_key" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."endpoint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."principal_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."request_digest", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."response_content_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."idempotency_key" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."endpoint", OLD."endpoint", 20000) OR
public.text_value_exceeds_bound(NEW."key", OLD."key", 20000) OR
public.text_value_exceeds_bound(NEW."principal_id", OLD."principal_id", 20000) OR
public.text_value_exceeds_bound(NEW."request_digest", OLD."request_digest", 20000) OR
public.text_value_exceeds_bound(NEW."response_content_type", OLD."response_content_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."import_record_map" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."external_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."object", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."origin", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_system", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."import_record_map" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."external_id", OLD."external_id", 20000) OR
public.text_value_exceeds_bound(NEW."object", OLD."object", 20000) OR
public.text_value_exceeds_bound(NEW."origin", OLD."origin", 20000) OR
public.text_value_exceeds_bound(NEW."source_system", OLD."source_system", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."import_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."approved_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."connector", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."draft_step", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."error", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."job_status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."phase", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."phase_object", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."import_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."approved_by", OLD."approved_by", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."connector", OLD."connector", 20000) OR
public.text_value_exceeds_bound(NEW."draft_step", OLD."draft_step", 20000) OR
public.text_value_exceeds_bound(NEW."error", OLD."error", 20000) OR
public.text_value_exceeds_bound(NEW."job_status", OLD."job_status", 20000) OR
public.text_value_exceeds_bound(NEW."phase", OLD."phase", 20000) OR
public.text_value_exceeds_bound(NEW."phase_object", OLD."phase_object", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."source_ref", OLD."source_ref", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."intro_request" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."decision_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."fallback_policy", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."forwardable_note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."internal_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."note_generated_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."origin", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."recorded_channel", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."route_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."value_for_target", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."intro_request" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."decision_reason", OLD."decision_reason", 2000) OR
public.text_value_exceeds_bound(NEW."fallback_policy", OLD."fallback_policy", 20000) OR
public.text_value_exceeds_bound(NEW."forwardable_note", OLD."forwardable_note", 2000) OR
public.text_value_exceeds_bound(NEW."internal_reason", OLD."internal_reason", 2000) OR
public.text_value_exceeds_bound(NEW."note_generated_by", OLD."note_generated_by", 20000) OR
public.text_value_exceeds_bound(NEW."origin", OLD."origin", 20000) OR
public.text_value_exceeds_bound(NEW."recorded_channel", OLD."recorded_channel", 20000) OR
public.text_value_exceeds_bound(NEW."route_type", OLD."route_type", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."value_for_target", OLD."value_for_target", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."knowledge_chunk" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."chunk_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."embed_identity", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."text", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."knowledge_chunk" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."chunk_hash", OLD."chunk_hash", 20000) OR
public.text_value_exceeds_bound(NEW."embed_identity", OLD."embed_identity", 20000) OR
public.text_value_exceeds_bound(NEW."text", OLD."text", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."knowledge_corpus" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."description", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."managed_source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."topic_statement", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."knowledge_corpus" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."description", OLD."description", 20000) OR
public.text_value_exceeds_bound(NEW."managed_source", OLD."managed_source", 20000) OR
public.text_value_exceeds_bound(NEW."name", OLD."name", 120) OR
public.text_value_exceeds_bound(NEW."topic_statement", OLD."topic_statement", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."knowledge_document" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."checksum", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."content_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."filename", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."ingest_detail", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."ingest_status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."managed_source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."storage_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."knowledge_document" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."checksum", OLD."checksum", 20000) OR
public.text_value_exceeds_bound(NEW."content_type", OLD."content_type", 20000) OR
public.text_value_exceeds_bound(NEW."filename", OLD."filename", 20000) OR
public.text_value_exceeds_bound(NEW."ingest_detail", OLD."ingest_detail", 20000) OR
public.text_value_exceeds_bound(NEW."ingest_status", OLD."ingest_status", 20000) OR
public.text_value_exceeds_bound(NEW."managed_source", OLD."managed_source", 20000) OR
public.text_value_exceeds_bound(NEW."storage_key", OLD."storage_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."lead" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."candidate_company_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."company_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."disqualify_note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."email", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."email_domain_folded", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."full_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."industry", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."linkedin_url", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."origin", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."origin_detail", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."partner_accepted_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."phone", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."score_override_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_author_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."source_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_system", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status_set_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."title", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."lead" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."candidate_company_key", OLD."candidate_company_key", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."company_name", OLD."company_name", 120) OR
public.text_value_exceeds_bound(NEW."disqualify_note", OLD."disqualify_note", 2000) OR
public.text_value_exceeds_bound(NEW."email", OLD."email", 20000) OR
public.text_value_exceeds_bound(NEW."email_domain_folded", OLD."email_domain_folded", 20000) OR
public.text_value_exceeds_bound(NEW."full_name", OLD."full_name", 120) OR
public.text_value_exceeds_bound(NEW."industry", OLD."industry", 20000) OR
public.text_value_exceeds_bound(NEW."linkedin_url", OLD."linkedin_url", 20000) OR
public.text_value_exceeds_bound(NEW."origin", OLD."origin", 20000) OR
public.text_value_exceeds_bound(NEW."origin_detail", OLD."origin_detail", 20000) OR
public.text_value_exceeds_bound(NEW."partner_accepted_by", OLD."partner_accepted_by", 20000) OR
public.text_value_exceeds_bound(NEW."phone", OLD."phone", 20000) OR
public.text_value_exceeds_bound(NEW."score_override_reason", OLD."score_override_reason", 2000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."source_author_name", OLD."source_author_name", 120) OR
public.text_value_exceeds_bound(NEW."source_id", OLD."source_id", 20000) OR
public.text_value_exceeds_bound(NEW."source_system", OLD."source_system", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."status_set_by", OLD."status_set_by", 20000) OR
public.text_value_exceeds_bound(NEW."title", OLD."title", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."lead_disqualify_reason" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."shipped_label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."shipped_label_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."lead_disqualify_reason" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."label", OLD."label", 120) OR
public.text_value_exceeds_bound(NEW."shipped_label", OLD."shipped_label", 120) OR
public.text_value_exceeds_bound(NEW."shipped_label_key", OLD."shipped_label_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."lead_manual_signal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."band", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."factor", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."signal_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."superseded_by", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."lead_manual_signal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."band", OLD."band", 20000) OR
public.text_value_exceeds_bound(NEW."factor", OLD."factor", 20000) OR
public.text_value_exceeds_bound(NEW."reason", OLD."reason", 2000) OR
public.text_value_exceeds_bound(NEW."signal_kind", OLD."signal_kind", 20000) OR
public.text_value_exceeds_bound(NEW."superseded_by", OLD."superseded_by", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."lead_origin" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."intent", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."shipped_label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."shipped_label_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."lead_origin" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."intent", OLD."intent", 20000) OR
public.text_value_exceeds_bound(NEW."key", OLD."key", 20000) OR
public.text_value_exceeds_bound(NEW."label", OLD."label", 120) OR
public.text_value_exceeds_bound(NEW."shipped_label", OLD."shipped_label", 120) OR
public.text_value_exceeds_bound(NEW."shipped_label_key", OLD."shipped_label_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."lead_privacy_notice" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."failure", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."locale", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."template_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."lead_privacy_notice" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."failure", OLD."failure", 20000) OR
public.text_value_exceeds_bound(NEW."locale", OLD."locale", 20000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000) OR
public.text_value_exceeds_bound(NEW."template_key", OLD."template_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."lead_score_history" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."override_reason", NULL, 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."lead_score_history" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."override_reason", OLD."override_reason", 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."lead_source" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."intent", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."label", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."lead_source" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."intent", OLD."intent", 20000) OR
public.text_value_exceeds_bound(NEW."key", OLD."key", 20000) OR
public.text_value_exceeds_bound(NEW."label", OLD."label", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."legal_entity" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."address_city", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."address_line1", NULL, 500) OR
public.text_value_exceeds_bound(NEW."address_line2", NULL, 500) OR
public.text_value_exceeds_bound(NEW."address_postal_code", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."address_region", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."bank_account_holder", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."bic", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."country", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."default_language", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."display_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."iban", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."icp", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."imprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."legal_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."logo_object_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."offer_summary", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."register_court", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."register_number", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."signature_template", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."usp", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."value_proposition", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."vat_id", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."legal_entity" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."address_city", OLD."address_city", 20000) OR
public.text_value_exceeds_bound(NEW."address_line1", OLD."address_line1", 500) OR
public.text_value_exceeds_bound(NEW."address_line2", OLD."address_line2", 500) OR
public.text_value_exceeds_bound(NEW."address_postal_code", OLD."address_postal_code", 20000) OR
public.text_value_exceeds_bound(NEW."address_region", OLD."address_region", 20000) OR
public.text_value_exceeds_bound(NEW."bank_account_holder", OLD."bank_account_holder", 20000) OR
public.text_value_exceeds_bound(NEW."bic", OLD."bic", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."country", OLD."country", 20000) OR
public.text_value_exceeds_bound(NEW."default_language", OLD."default_language", 20000) OR
public.text_value_exceeds_bound(NEW."display_name", OLD."display_name", 120) OR
public.text_value_exceeds_bound(NEW."iban", OLD."iban", 20000) OR
public.text_value_exceeds_bound(NEW."icp", OLD."icp", 20000) OR
public.text_value_exceeds_bound(NEW."imprint", OLD."imprint", 20000) OR
public.text_value_exceeds_bound(NEW."legal_name", OLD."legal_name", 120) OR
public.text_value_exceeds_bound(NEW."logo_object_key", OLD."logo_object_key", 20000) OR
public.text_value_exceeds_bound(NEW."offer_summary", OLD."offer_summary", 20000) OR
public.text_value_exceeds_bound(NEW."register_court", OLD."register_court", 20000) OR
public.text_value_exceeds_bound(NEW."register_number", OLD."register_number", 20000) OR
public.text_value_exceeds_bound(NEW."signature_template", OLD."signature_template", 20000) OR
public.text_value_exceeds_bound(NEW."usp", OLD."usp", 20000) OR
public.text_value_exceeds_bound(NEW."value_proposition", OLD."value_proposition", 20000) OR
public.text_value_exceeds_bound(NEW."vat_id", OLD."vat_id", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."legal_entity_member" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."assigned_by", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."legal_entity_member" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."assigned_by", OLD."assigned_by", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."legal_review" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."comments", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."legal_review" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."comments", OLD."comments", 20000) OR
public.text_value_exceeds_bound(NEW."note", OLD."note", 2000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."linkedin_account" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."profile_url", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."linkedin_account" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."profile_url", OLD."profile_url", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."linkedin_connection" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."company_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."email", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."full_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."match_status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."normalized_company", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."normalized_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."position", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."profile_url", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider_member_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."linkedin_connection" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."company_name", OLD."company_name", 120) OR
public.text_value_exceeds_bound(NEW."email", OLD."email", 20000) OR
public.text_value_exceeds_bound(NEW."full_name", OLD."full_name", 120) OR
public.text_value_exceeds_bound(NEW."match_status", OLD."match_status", 20000) OR
public.text_value_exceeds_bound(NEW."normalized_company", OLD."normalized_company", 20000) OR
public.text_value_exceeds_bound(NEW."normalized_name", OLD."normalized_name", 120) OR
public.text_value_exceeds_bound(NEW."position", OLD."position", 20000) OR
public.text_value_exceeds_bound(NEW."profile_url", OLD."profile_url", 20000) OR
public.text_value_exceeds_bound(NEW."provider_member_ref", OLD."provider_member_ref", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."list" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."entity_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."list_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."purpose", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."sharing", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."list" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."entity_type", OLD."entity_type", 20000) OR
public.text_value_exceeds_bound(NEW."list_type", OLD."list_type", 20000) OR
public.text_value_exceeds_bound(NEW."name", OLD."name", 120) OR
public.text_value_exceeds_bound(NEW."purpose", OLD."purpose", 20000) OR
public.text_value_exceeds_bound(NEW."sharing", OLD."sharing", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."list_evaluation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."outcome", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."list_evaluation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."list_live_member" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."entity_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."list_live_member" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."entity_type", OLD."entity_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."list_member" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."added_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."note", NULL, 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."list_member" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."added_by", OLD."added_by", 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", OLD."entity_type", 20000) OR
public.text_value_exceeds_bound(NEW."note", OLD."note", 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."list_member_event" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."action", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."actor", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."member_note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."reason", NULL, 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."list_member_event" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."action", OLD."action", 20000) OR
public.text_value_exceeds_bound(NEW."actor", OLD."actor", 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", OLD."entity_type", 20000) OR
public.text_value_exceeds_bound(NEW."member_note", OLD."member_note", 2000) OR
public.text_value_exceeds_bound(NEW."note", OLD."note", 2000) OR
public.text_value_exceeds_bound(NEW."reason", OLD."reason", 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."list_revision" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."changed_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."purpose", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."sharing", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."list_revision" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."changed_by", OLD."changed_by", 20000) OR
public.text_value_exceeds_bound(NEW."name", OLD."name", 120) OR
public.text_value_exceeds_bound(NEW."purpose", OLD."purpose", 20000) OR
public.text_value_exceeds_bound(NEW."sharing", OLD."sharing", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."mail_draft" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."anchor_type", NULL, 20000) OR
public.text_values_exceed_bound(NEW."bcc_addresses", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."body", NULL, 20000) OR
public.text_values_exceed_bound(NEW."cc_addresses", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."communication_context", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."html_body", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."scheduled_tz", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject", NULL, 120) OR
public.text_values_exceed_bound(NEW."to_addresses", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."mail_draft" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."anchor_type", OLD."anchor_type", 20000) OR
public.text_values_exceed_bound(NEW."bcc_addresses", OLD."bcc_addresses", 20000) OR
public.text_value_exceeds_bound(NEW."body", OLD."body", 20000) OR
public.text_values_exceed_bound(NEW."cc_addresses", OLD."cc_addresses", 20000) OR
public.text_value_exceeds_bound(NEW."communication_context", OLD."communication_context", 20000) OR
public.text_value_exceeds_bound(NEW."html_body", OLD."html_body", 20000) OR
public.text_value_exceeds_bound(NEW."scheduled_tz", OLD."scheduled_tz", 20000) OR
public.text_value_exceeds_bound(NEW."subject", OLD."subject", 120) OR
public.text_values_exceed_bound(NEW."to_addresses", OLD."to_addresses", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."maskable_field" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."field", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."object", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."maskable_field" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."field", OLD."field", 20000) OR
public.text_value_exceeds_bound(NEW."object", OLD."object", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."meeting_calendar_episode" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."provider", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."meeting_calendar_episode" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."meeting_invitation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."attention_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."calendar_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."command", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."management_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."management_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reminder_status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."meeting_invitation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."attention_reason", OLD."attention_reason", 2000) OR
public.text_value_exceeds_bound(NEW."calendar_id", OLD."calendar_id", 20000) OR
public.text_value_exceeds_bound(NEW."command", OLD."command", 20000) OR
public.text_value_exceeds_bound(NEW."management_hash", OLD."management_hash", 20000) OR
public.text_value_exceeds_bound(NEW."management_ref", OLD."management_ref", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_value_exceeds_bound(NEW."reminder_status", OLD."reminder_status", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."meeting_proposal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."link_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."token_hash", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."meeting_proposal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."link_ref", OLD."link_ref", 20000) OR
public.text_value_exceeds_bound(NEW."token_hash", OLD."token_hash", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."meeting_readiness" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."agenda_fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."participants_fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."revision", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."meeting_readiness" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."agenda_fingerprint", OLD."agenda_fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."participants_fingerprint", OLD."participants_fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."revision", OLD."revision", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."mfa_challenge_spent" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."jti", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."mfa_challenge_spent" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."jti", OLD."jti", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."mfa_recovery_code" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."code_hash", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."mfa_recovery_code" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."code_hash", OLD."code_hash", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."notice" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."body", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."dedupe_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."email_error", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject", NULL, 120) OR
public.text_value_exceeds_bound(NEW."target_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."notice" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."body", OLD."body", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."dedupe_key", OLD."dedupe_key", 20000) OR
public.text_value_exceeds_bound(NEW."email_error", OLD."email_error", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."subject", OLD."subject", 120) OR
public.text_value_exceeds_bound(NEW."target_type", OLD."target_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."notification_digest_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."mail_error", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."notification_digest_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."mail_error", OLD."mail_error", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."notification_preference" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."class", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."delivery", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."notification_preference" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."class", OLD."class", 20000) OR
public.text_value_exceeds_bound(NEW."delivery", OLD."delivery", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."oauth_authorization_code" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."client_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."code_challenge", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."code_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."redirect_uri", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."resource", NULL, 20000) OR
public.text_values_exceed_bound(NEW."scopes", NULL, 20000) OR
public.text_values_exceed_bound(NEW."work_areas", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."oauth_authorization_code" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."client_id", OLD."client_id", 20000) OR
public.text_value_exceeds_bound(NEW."code_challenge", OLD."code_challenge", 20000) OR
public.text_value_exceeds_bound(NEW."code_hash", OLD."code_hash", 20000) OR
public.text_value_exceeds_bound(NEW."redirect_uri", OLD."redirect_uri", 20000) OR
public.text_value_exceeds_bound(NEW."resource", OLD."resource", 20000) OR
public.text_values_exceed_bound(NEW."scopes", OLD."scopes", 20000) OR
public.text_values_exceed_bound(NEW."work_areas", OLD."work_areas", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."oauth_client" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."client_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."client_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."client_secret_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."created_via", NULL, 20000) OR
public.text_values_exceed_bound(NEW."redirect_uris", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."oauth_client" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."client_id", OLD."client_id", 20000) OR
public.text_value_exceeds_bound(NEW."client_name", OLD."client_name", 120) OR
public.text_value_exceeds_bound(NEW."client_secret_hash", OLD."client_secret_hash", 20000) OR
public.text_value_exceeds_bound(NEW."created_via", OLD."created_via", 20000) OR
public.text_values_exceed_bound(NEW."redirect_uris", OLD."redirect_uris", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."oauth_grant" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."client_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."resource", NULL, 20000) OR
public.text_values_exceed_bound(NEW."scopes", NULL, 20000) OR
public.text_values_exceed_bound(NEW."work_areas", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."oauth_grant" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."client_id", OLD."client_id", 20000) OR
public.text_value_exceeds_bound(NEW."resource", OLD."resource", 20000) OR
public.text_values_exceed_bound(NEW."scopes", OLD."scopes", 20000) OR
public.text_values_exceed_bound(NEW."work_areas", OLD."work_areas", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."oauth_refresh_token" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."token_hash", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."oauth_refresh_token" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."token_hash", OLD."token_hash", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."offer" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."intro_text", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."language", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."offer_number", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."pdf_asset_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."terms_text", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."offer" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."intro_text", OLD."intro_text", 20000) OR
public.text_value_exceeds_bound(NEW."language", OLD."language", 20000) OR
public.text_value_exceeds_bound(NEW."offer_number", OLD."offer_number", 20000) OR
public.text_value_exceeds_bound(NEW."pdf_asset_ref", OLD."pdf_asset_ref", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."terms_text", OLD."terms_text", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."offer_line_item" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."billing_model", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."description", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."proposal_state", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."tax_category", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."unit", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."offer_line_item" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."billing_model", OLD."billing_model", 20000) OR
public.text_value_exceeds_bound(NEW."description", OLD."description", 20000) OR
public.text_value_exceeds_bound(NEW."proposal_state", OLD."proposal_state", 20000) OR
public.text_value_exceeds_bound(NEW."tax_category", OLD."tax_category", 20000) OR
public.text_value_exceeds_bound(NEW."unit", OLD."unit", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."offer_template" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."locale", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."name", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."offer_template" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."locale", OLD."locale", 20000) OR
public.text_value_exceeds_bound(NEW."name", OLD."name", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."onboarding_wizard_state" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."path", NULL, 20000) OR
public.text_values_exceed_bound(NEW."selected_fact_keys", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_mode", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."step", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."website_url", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."onboarding_wizard_state" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."path", OLD."path", 20000) OR
public.text_values_exceed_bound(NEW."selected_fact_keys", OLD."selected_fact_keys", 20000) OR
public.text_value_exceeds_bound(NEW."source_mode", OLD."source_mode", 20000) OR
public.text_value_exceeds_bound(NEW."step", OLD."step", 20000) OR
public.text_value_exceeds_bound(NEW."website_url", OLD."website_url", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."partner" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."activated_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."activation_basis", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."cert_status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."margin_tier", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."next_step", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."partner_fit_override_reason", NULL, 2000) OR
public.text_values_exceed_bound(NEW."partner_kinds", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."relationship_stage", NULL, 20000) OR
public.text_values_exceed_bound(NEW."served_segments", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."partner" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."activated_by", OLD."activated_by", 20000) OR
public.text_value_exceeds_bound(NEW."activation_basis", OLD."activation_basis", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."cert_status", OLD."cert_status", 20000) OR
public.text_value_exceeds_bound(NEW."margin_tier", OLD."margin_tier", 20000) OR
public.text_value_exceeds_bound(NEW."next_step", OLD."next_step", 20000) OR
public.text_value_exceeds_bound(NEW."partner_fit_override_reason", OLD."partner_fit_override_reason", 2000) OR
public.text_values_exceed_bound(NEW."partner_kinds", OLD."partner_kinds", 20000) OR
public.text_value_exceeds_bound(NEW."relationship_stage", OLD."relationship_stage", 20000) OR
public.text_values_exceed_bound(NEW."served_segments", OLD."served_segments", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."partner_stage" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."meaning", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."shipped_label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."shipped_label_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."partner_stage" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."key", OLD."key", 20000) OR
public.text_value_exceeds_bound(NEW."label", OLD."label", 120) OR
public.text_value_exceeds_bound(NEW."meaning", OLD."meaning", 20000) OR
public.text_value_exceeds_bound(NEW."shipped_label", OLD."shipped_label", 120) OR
public.text_value_exceeds_bound(NEW."shipped_label_key", OLD."shipped_label_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."passport" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."label", NULL, 120) OR
public.text_values_exceed_bound(NEW."scopes", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."token_hash", NULL, 20000) OR
public.text_values_exceed_bound(NEW."work_areas", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."passport" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."label", OLD."label", 120) OR
public.text_values_exceed_bound(NEW."scopes", OLD."scopes", 20000) OR
public.text_value_exceeds_bound(NEW."token_hash", OLD."token_hash", 20000) OR
public.text_values_exceed_bound(NEW."work_areas", OLD."work_areas", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."pipeline" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."name", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."pipeline" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."name", OLD."name", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."preference_token" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."revoked_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."token", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."token_hash", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."preference_token" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."revoked_reason", OLD."revoked_reason", 2000) OR
public.text_value_exceeds_bound(NEW."token", OLD."token", 20000) OR
public.text_value_exceeds_bound(NEW."token_hash", OLD."token_hash", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."privacy_notice_case" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."allowed_routes", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."blocked_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."resolution_note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."rule", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_evidence_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_review_note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."privacy_notice_case" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."allowed_routes", OLD."allowed_routes", 20000) OR
public.text_value_exceeds_bound(NEW."blocked_reason", OLD."blocked_reason", 2000) OR
public.text_value_exceeds_bound(NEW."resolution_note", OLD."resolution_note", 2000) OR
public.text_value_exceeds_bound(NEW."rule", OLD."rule", 20000) OR
public.text_value_exceeds_bound(NEW."source_evidence_kind", OLD."source_evidence_kind", 20000) OR
public.text_value_exceeds_bound(NEW."source_kind", OLD."source_kind", 20000) OR
public.text_value_exceeds_bound(NEW."source_review_note", OLD."source_review_note", 2000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."privacy_request_proof" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."completed_by_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."completed_by_type", NULL, 20000) OR
public.text_values_exceed_bound(NEW."erased_uncounted", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."identity_check", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."request_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."request_number", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."requester_email", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."result", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."privacy_request_proof" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."completed_by_id", OLD."completed_by_id", 20000) OR
public.text_value_exceeds_bound(NEW."completed_by_type", OLD."completed_by_type", 20000) OR
public.text_values_exceed_bound(NEW."erased_uncounted", OLD."erased_uncounted", 20000) OR
public.text_value_exceeds_bound(NEW."identity_check", OLD."identity_check", 20000) OR
public.text_value_exceeds_bound(NEW."request_kind", OLD."request_kind", 20000) OR
public.text_value_exceeds_bound(NEW."request_number", OLD."request_number", 20000) OR
public.text_value_exceeds_bound(NEW."requester_email", OLD."requester_email", 20000) OR
public.text_value_exceeds_bound(NEW."result", OLD."result", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."product" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."billing_model", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."component_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."description", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."sku", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."tax_category", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."unit", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."product" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."billing_model", OLD."billing_model", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."component_kind", OLD."component_kind", 20000) OR
public.text_value_exceeds_bound(NEW."description", OLD."description", 20000) OR
public.text_value_exceeds_bound(NEW."name", OLD."name", 120) OR
public.text_value_exceeds_bound(NEW."sku", OLD."sku", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."tax_category", OLD."tax_category", 20000) OR
public.text_value_exceeds_bound(NEW."unit", OLD."unit", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."product_translation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."description", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."language", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."name", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."product_translation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."description", OLD."description", 20000) OR
public.text_value_exceeds_bound(NEW."language", OLD."language", 20000) OR
public.text_value_exceeds_bound(NEW."name", OLD."name", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."project" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."closed_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."description", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."phase", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_author_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."source_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_system", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."project" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."closed_reason", OLD."closed_reason", 2000) OR
public.text_value_exceeds_bound(NEW."description", OLD."description", 20000) OR
public.text_value_exceeds_bound(NEW."key", OLD."key", 20000) OR
public.text_value_exceeds_bound(NEW."name", OLD."name", 120) OR
public.text_value_exceeds_bound(NEW."phase", OLD."phase", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."source_author_name", OLD."source_author_name", 120) OR
public.text_value_exceeds_bound(NEW."source_id", OLD."source_id", 20000) OR
public.text_value_exceeds_bound(NEW."source_system", OLD."source_system", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."project_health_assessment" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_author", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."project_health_assessment" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."note", OLD."note", 2000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."source_author", OLD."source_author", 20000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."project_phase_history" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."changed_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."from_phase", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."to_phase", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."project_phase_history" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."changed_by", OLD."changed_by", 20000) OR
public.text_value_exceeds_bound(NEW."from_phase", OLD."from_phase", 20000) OR
public.text_value_exceeds_bound(NEW."reason", OLD."reason", 2000) OR
public.text_value_exceeds_bound(NEW."to_phase", OLD."to_phase", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."provider_applied_field" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."applied_value", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."target_field", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."target_table", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."provider_applied_field" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."applied_value", OLD."applied_value", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_value_exceeds_bound(NEW."target_field", OLD."target_field", 20000) OR
public.text_value_exceeds_bound(NEW."target_table", OLD."target_table", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."provider_connection" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."categories", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."credential_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."last_safe_status_code", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."mode", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."preset", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."provider_connection" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."categories", OLD."categories", 20000) OR
public.text_value_exceeds_bound(NEW."credential_ref", OLD."credential_ref", 20000) OR
public.text_value_exceeds_bound(NEW."last_safe_status_code", OLD."last_safe_status_code", 20000) OR
public.text_value_exceeds_bound(NEW."mode", OLD."mode", 20000) OR
public.text_value_exceeds_bound(NEW."preset", OLD."preset", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."provider_connection_budget" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."pool", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."provider_connection_budget" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."pool", OLD."pool", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."provider_employment_resolution" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."episode_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."provider_employment_resolution" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."episode_key", OLD."episode_key", 20000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."provider_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."input_fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."last_safe_status_code", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider_job_id", NULL, 20000) OR
public.text_values_exceed_bound(NEW."requested_categories", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."skip_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."trigger", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."provider_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."input_fingerprint", OLD."input_fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."last_safe_status_code", OLD."last_safe_status_code", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000) OR
public.text_value_exceeds_bound(NEW."provider_job_id", OLD."provider_job_id", 20000) OR
public.text_values_exceed_bound(NEW."requested_categories", OLD."requested_categories", 20000) OR
public.text_value_exceeds_bound(NEW."skip_reason", OLD."skip_reason", 2000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000) OR
public.text_value_exceeds_bound(NEW."subject_kind", OLD."subject_kind", 20000) OR
public.text_value_exceeds_bound(NEW."trigger", OLD."trigger", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."provider_run_category" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."category", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."provider", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."provider_run_category" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."category", OLD."category", 20000) OR
public.text_value_exceeds_bound(NEW."provider", OLD."provider", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."provider_run_reservation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."pool", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."provider_run_reservation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."pool", OLD."pool", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."raw_capture" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."parts_slim_outcome", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_system", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."raw_capture" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."parts_slim_outcome", OLD."parts_slim_outcome", 20000) OR
public.text_value_exceeds_bound(NEW."source_id", OLD."source_id", 20000) OR
public.text_value_exceeds_bound(NEW."source_system", OLD."source_system", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."record_assignment" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."record_assignment" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."record_grant" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."access", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."record_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."subject_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."record_grant" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."access", OLD."access", 20000) OR
public.text_value_exceeds_bound(NEW."reason", OLD."reason", 2000) OR
public.text_value_exceeds_bound(NEW."record_type", OLD."record_type", 20000) OR
public.text_value_exceeds_bound(NEW."subject_type", OLD."subject_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."record_role" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."assignee_kinds", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."label", NULL, 120) OR
public.text_values_exceed_bound(NEW."record_types", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."shipped_label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."shipped_label_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."record_role" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."assignee_kinds", OLD."assignee_kinds", 20000) OR
public.text_value_exceeds_bound(NEW."key", OLD."key", 20000) OR
public.text_value_exceeds_bound(NEW."label", OLD."label", 120) OR
public.text_values_exceed_bound(NEW."record_types", OLD."record_types", 20000) OR
public.text_value_exceeds_bound(NEW."shipped_label", OLD."shipped_label", 120) OR
public.text_value_exceeds_bound(NEW."shipped_label_key", OLD."shipped_label_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."relationship" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."employment_status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."ended_precision", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."role", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."started_precision", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."relationship" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."employment_status", OLD."employment_status", 20000) OR
public.text_value_exceeds_bound(NEW."ended_precision", OLD."ended_precision", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."role", OLD."role", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."started_precision", OLD."started_precision", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."relationship_nudge_dismissal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."set_by", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."relationship_nudge_dismissal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."set_by", OLD."set_by", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."report_definition" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."audience", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."description", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."name", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."report_definition" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."audience", OLD."audience", 20000) OR
public.text_value_exceeds_bound(NEW."description", OLD."description", 20000) OR
public.text_value_exceeds_bound(NEW."name", OLD."name", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."report_edition" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."publication_audience", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."report_edition" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."publication_audience", OLD."publication_audience", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."report_edition_contribution" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."context_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."contribution_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."group_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."metric", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."report_edition_contribution" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."context_id", OLD."context_id", 20000) OR
public.text_value_exceeds_bound(NEW."contribution_key", OLD."contribution_key", 20000) OR
public.text_value_exceeds_bound(NEW."group_key", OLD."group_key", 20000) OR
public.text_value_exceeds_bound(NEW."metric", OLD."metric", 20000) OR
public.text_value_exceeds_bound(NEW."source_type", OLD."source_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."report_edition_question" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."card_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."entity", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason", NULL, 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."report_edition_question" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."card_id", OLD."card_id", 20000) OR
public.text_value_exceeds_bound(NEW."entity", OLD."entity", 20000) OR
public.text_value_exceeds_bound(NEW."reason", OLD."reason", 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."report_edition_question_source" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."card_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."report_edition_question_source" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."card_id", OLD."card_id", 20000) OR
public.text_value_exceeds_bound(NEW."source_type", OLD."source_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."report_execution" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."execution_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."request_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."report_execution" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."execution_key", OLD."execution_key", 20000) OR
public.text_value_exceeds_bound(NEW."reason", OLD."reason", 2000) OR
public.text_value_exceeds_bound(NEW."request_hash", OLD."request_hash", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."report_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."schema_version", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."report_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."schema_version", OLD."schema_version", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."report_schedule" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."last_status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."timezone", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."report_schedule" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."last_status", OLD."last_status", 20000) OR
public.text_value_exceeds_bound(NEW."timezone", OLD."timezone", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."restore_drill" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."notes", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."operator", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."outcome", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."restore_drill" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."notes", OLD."notes", 2000) OR
public.text_value_exceeds_bound(NEW."operator", OLD."operator", 20000) OR
public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."retention_policy" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."action", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."category", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."lawful_basis", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."object_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."retention_policy" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."action", OLD."action", 20000) OR
public.text_value_exceeds_bound(NEW."category", OLD."category", 20000) OR
public.text_value_exceeds_bound(NEW."lawful_basis", OLD."lawful_basis", 20000) OR
public.text_value_exceeds_bound(NEW."object_type", OLD."object_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."role" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."name", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."role" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."key", OLD."key", 20000) OR
public.text_value_exceeds_bound(NEW."name", OLD."name", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."runner_job" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."agent_spec", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."last_error", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."trigger_ref", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."runner_job" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."agent_spec", OLD."agent_spec", 20000) OR
public.text_value_exceeds_bound(NEW."last_error", OLD."last_error", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."trigger_ref", OLD."trigger_ref", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."sales_goal_plan_revision" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."origin", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason", NULL, 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."sales_goal_plan_revision" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."origin", OLD."origin", 20000) OR
public.text_value_exceeds_bound(NEW."reason", OLD."reason", 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."sales_initiative" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."description", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."objective_metric", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."sales_initiative" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."description", OLD."description", 20000) OR
public.text_value_exceeds_bound(NEW."name", OLD."name", 120) OR
public.text_value_exceeds_bound(NEW."objective_metric", OLD."objective_metric", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."sales_initiative_baseline" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."sales_initiative_baseline" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."sales_initiative_member" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."record_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."sales_initiative_member" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."reason", OLD."reason", 2000) OR
public.text_value_exceeds_bound(NEW."record_type", OLD."record_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."sales_initiative_revision" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."created_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason", NULL, 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."sales_initiative_revision" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."created_by", OLD."created_by", 20000) OR
public.text_value_exceeds_bound(NEW."reason", OLD."reason", 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."sales_target" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."metric", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."period_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."scope_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."unit", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."sales_target" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."metric", OLD."metric", 20000) OR
public.text_value_exceeds_bound(NEW."period_kind", OLD."period_kind", 20000) OR
public.text_value_exceeds_bound(NEW."scope_kind", OLD."scope_kind", 20000) OR
public.text_value_exceeds_bound(NEW."unit", OLD."unit", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."saved_view" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."resource", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."shared_scope", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."saved_view" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."name", OLD."name", 120) OR
public.text_value_exceeds_bound(NEW."resource", OLD."resource", 20000) OR
public.text_value_exceeds_bound(NEW."shared_scope", OLD."shared_scope", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."scheduled_send" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."agent_actor_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."held_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."origin_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."principal_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."scheduled_tz", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."scheduled_send" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."agent_actor_id", OLD."agent_actor_id", 20000) OR
public.text_value_exceeds_bound(NEW."held_reason", OLD."held_reason", 2000) OR
public.text_value_exceeds_bound(NEW."origin_kind", OLD."origin_kind", 20000) OR
public.text_value_exceeds_bound(NEW."principal_kind", OLD."principal_kind", 20000) OR
public.text_value_exceeds_bound(NEW."scheduled_tz", OLD."scheduled_tz", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."sdr_handoff" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."reason_applies_to", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."sdr_handoff" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."note", OLD."note", 2000) OR
public.text_value_exceeds_bound(NEW."reason_applies_to", OLD."reason_applies_to", 2000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."sdr_handoff_event" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."actor", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."reason_applies_to", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."to_status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."sdr_handoff_event" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."actor", OLD."actor", 20000) OR
public.text_value_exceeds_bound(NEW."note", OLD."note", 2000) OR
public.text_value_exceeds_bound(NEW."reason_applies_to", OLD."reason_applies_to", 2000) OR
public.text_value_exceeds_bound(NEW."to_status", OLD."to_status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."sdr_handoff_reason" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."applies_to", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."label", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."sdr_handoff_reason" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."applies_to", OLD."applies_to", 20000) OR
public.text_value_exceeds_bound(NEW."label", OLD."label", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."session" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."token_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."user_agent", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."session" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."token_hash", OLD."token_hash", 20000) OR
public.text_value_exceeds_bound(NEW."user_agent", OLD."user_agent", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."setting" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."setting" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."key", OLD."key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."setup_token" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."token_hash", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."setup_token" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."token_hash", OLD."token_hash", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."signal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."fingerprint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."raw_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."resolution_state", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."severity", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_channel", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."summary", NULL, 500) OR
public.text_value_exceeds_bound(NEW."visibility", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."signal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", OLD."entity_type", 20000) OR
public.text_value_exceeds_bound(NEW."fingerprint", OLD."fingerprint", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."raw_ref", OLD."raw_ref", 20000) OR
public.text_value_exceeds_bound(NEW."resolution_state", OLD."resolution_state", 20000) OR
public.text_value_exceeds_bound(NEW."severity", OLD."severity", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."source_channel", OLD."source_channel", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."summary", OLD."summary", 500) OR
public.text_value_exceeds_bound(NEW."visibility", OLD."visibility", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."signal_resolution" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."matched_on", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."outcome", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."signal_resolution" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."matched_on", OLD."matched_on", 20000) OR
public.text_value_exceeds_bound(NEW."note", OLD."note", 2000) OR
public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."signal_thread_scan" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."thread_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."signal_thread_scan" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."thread_key", OLD."thread_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."site_read" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."logo_icon_object_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."logo_icon_origin", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."logo_object_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."logo_origin", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."phase", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."proposal_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."requested_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."seed_url", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status_code", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status_detail", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."stopped_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."target_kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."site_read" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."logo_icon_object_key", OLD."logo_icon_object_key", 20000) OR
public.text_value_exceeds_bound(NEW."logo_icon_origin", OLD."logo_icon_origin", 20000) OR
public.text_value_exceeds_bound(NEW."logo_object_key", OLD."logo_object_key", 20000) OR
public.text_value_exceeds_bound(NEW."logo_origin", OLD."logo_origin", 20000) OR
public.text_value_exceeds_bound(NEW."phase", OLD."phase", 20000) OR
public.text_value_exceeds_bound(NEW."proposal_hash", OLD."proposal_hash", 20000) OR
public.text_value_exceeds_bound(NEW."requested_by", OLD."requested_by", 20000) OR
public.text_value_exceeds_bound(NEW."seed_url", OLD."seed_url", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."status_code", OLD."status_code", 20000) OR
public.text_value_exceeds_bound(NEW."status_detail", OLD."status_detail", 20000) OR
public.text_value_exceeds_bound(NEW."stopped_reason", OLD."stopped_reason", 2000) OR
public.text_value_exceeds_bound(NEW."target_kind", OLD."target_kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."stage" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."semantic", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."stage" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."name", OLD."name", 120) OR
public.text_value_exceeds_bound(NEW."semantic", OLD."semantic", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."stage_exit_criterion" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."hint", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."label", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."stage_exit_criterion" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."hint", OLD."hint", 20000) OR
public.text_value_exceeds_bound(NEW."key", OLD."key", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."label", OLD."label", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."stage_progression_outcome" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."evidence_kinds", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."outcome", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."rejection_reason", NULL, 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."stage_progression_outcome" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."evidence_kinds", OLD."evidence_kinds", 20000) OR
public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000) OR
public.text_value_exceeds_bound(NEW."rejection_reason", OLD."rejection_reason", 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."stage_progression_policy" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."mode", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."suspended_reason", NULL, 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."stage_progression_policy" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."mode", OLD."mode", 20000) OR
public.text_value_exceeds_bound(NEW."suspended_reason", OLD."suspended_reason", 2000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."stored_object_intent" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."storage_key", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."stored_object_intent" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."storage_key", OLD."storage_key", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."suggestion_dismissal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."fingerprint", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."suggestion_dismissal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."fingerprint", OLD."fingerprint", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."system_log" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."action", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."actor_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."actor_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."system_log" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."action", OLD."action", 20000) OR
public.text_value_exceeds_bound(NEW."actor_id", OLD."actor_id", 20000) OR
public.text_value_exceeds_bound(NEW."actor_type", OLD."actor_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."tag" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."color", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."description", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."name", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."tag" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."color", OLD."color", 20000) OR
public.text_value_exceeds_bound(NEW."description", OLD."description", 20000) OR
public.text_value_exceeds_bound(NEW."name", OLD."name", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."tag_interest_read" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."content_digest", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."vocabulary_digest", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."tag_interest_read" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."content_digest", OLD."content_digest", 20000) OR
public.text_value_exceeds_bound(NEW."vocabulary_digest", OLD."vocabulary_digest", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."tag_suggestion" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."tag_suggestion" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."taggable" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."assigned_by_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."taggable" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."assigned_by_kind", OLD."assigned_by_kind", 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", OLD."entity_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."tax_rate" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."category", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."country", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."tax_rate" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."category", OLD."category", 20000) OR
public.text_value_exceeds_bound(NEW."country", OLD."country", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."team" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."name", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."team" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."name", OLD."name", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."team_weekly_review" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."summary", NULL, 500) OR
public.text_value_exceeds_bound(NEW."summary_language", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."team_name", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."team_weekly_review" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."summary", OLD."summary", 500) OR
public.text_value_exceeds_bound(NEW."summary_language", OLD."summary_language", 20000) OR
public.text_value_exceeds_bound(NEW."team_name", OLD."team_name", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."team_weekly_review_coverage" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."absent", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."basis", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."period_kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."team_weekly_review_coverage" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."absent", OLD."absent", 20000) OR
public.text_value_exceeds_bound(NEW."basis", OLD."basis", 20000) OR
public.text_value_exceeds_bound(NEW."period_kind", OLD."period_kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."team_weekly_review_member" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."display_name", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."team_weekly_review_member" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."display_name", OLD."display_name", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."team_weekly_review_outlook" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."forward_measure", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."period_kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."team_weekly_review_outlook" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."forward_measure", OLD."forward_measure", 20000) OR
public.text_value_exceeds_bound(NEW."period_kind", OLD."period_kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."team_weekly_review_rep" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."display_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."focus_kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."focus_label", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."team_weekly_review_rep" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."display_name", OLD."display_name", 120) OR
public.text_value_exceeds_bound(NEW."focus_kind", OLD."focus_kind", 20000) OR
public.text_value_exceeds_bound(NEW."focus_label", OLD."focus_label", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."team_weekly_review_rep_funnel" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."next_stage_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."pipeline_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."stage_name", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."team_weekly_review_rep_funnel" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."next_stage_name", OLD."next_stage_name", 120) OR
public.text_value_exceeds_bound(NEW."pipeline_name", OLD."pipeline_name", 120) OR
public.text_value_exceeds_bound(NEW."stage_name", OLD."stage_name", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."team_weekly_review_rep_pattern" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."label", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."team_weekly_review_rep_pattern" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."label", OLD."label", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."technical_lookup_cache" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."query", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."record_kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."technical_lookup_cache" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."query", OLD."query", 20000) OR
public.text_value_exceeds_bound(NEW."record_kind", OLD."record_kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."transcript_read" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."requested_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status_detail", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."transcript_read" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."requested_by", OLD."requested_by", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."status_detail", OLD."status_detail", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."user_mfa" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."secret_ref", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."user_mfa" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."secret_ref", OLD."secret_ref", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."user_record_view" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."entity_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."user_record_view" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."entity_type", OLD."entity_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."vault_secret" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."ref", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."vault_secret" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."ref", OLD."ref", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."voice_build" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."candidate_action", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."source_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."stage", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status_code", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status_detail", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."voice_build" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."candidate_action", OLD."candidate_action", 20000) OR
public.text_value_exceeds_bound(NEW."reason", OLD."reason", 2000) OR
public.text_value_exceeds_bound(NEW."source_hash", OLD."source_hash", 20000) OR
public.text_value_exceeds_bound(NEW."stage", OLD."stage", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."status_code", OLD."status_code", 20000) OR
public.text_value_exceeds_bound(NEW."status_detail", OLD."status_detail", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."voice_corpus_source" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."content", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."content_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."exclusion_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."extractor_version", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."origin", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."register", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."source_ref", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."voice_corpus_source" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."content", OLD."content", 20000) OR
public.text_value_exceeds_bound(NEW."content_hash", OLD."content_hash", 20000) OR
public.text_value_exceeds_bound(NEW."exclusion_reason", OLD."exclusion_reason", 2000) OR
public.text_value_exceeds_bound(NEW."extractor_version", OLD."extractor_version", 20000) OR
public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."origin", OLD."origin", 20000) OR
public.text_value_exceeds_bound(NEW."register", OLD."register", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."source_label", OLD."source_label", 120) OR
public.text_value_exceeds_bound(NEW."source_ref", OLD."source_ref", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."voice_learning_signal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."final_captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."final_text", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."generated_original", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."outcome", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."voice_learning_signal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."final_captured_by", OLD."final_captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."final_text", OLD."final_text", 20000) OR
public.text_value_exceeds_bound(NEW."generated_original", OLD."generated_original", 20000) OR
public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."voice_profile" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."active_source_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."model_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."personality_md", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."scope", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."voice_profile_md", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."voice_profile" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."active_source_hash", OLD."active_source_hash", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."model_ref", OLD."model_ref", 20000) OR
public.text_value_exceeds_bound(NEW."personality_md", OLD."personality_md", 20000) OR
public.text_value_exceeds_bound(NEW."scope", OLD."scope", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."voice_profile_md", OLD."voice_profile_md", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."voice_profile_delta" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."activation_outcome", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."classification", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."voice_profile_delta" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."activation_outcome", OLD."activation_outcome", 20000) OR
public.text_value_exceeds_bound(NEW."classification", OLD."classification", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."voice_profile_version" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."activation_policy_version", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."builder_version", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."model_name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."model_provider", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."reason", NULL, 2000) OR
public.text_values_exceed_bound(NEW."review_reasons", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source_hash", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."voice_profile_md", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."voice_profile_version" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."activation_policy_version", OLD."activation_policy_version", 20000) OR
public.text_value_exceeds_bound(NEW."builder_version", OLD."builder_version", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."model_name", OLD."model_name", 120) OR
public.text_value_exceeds_bound(NEW."model_provider", OLD."model_provider", 20000) OR
public.text_value_exceeds_bound(NEW."reason", OLD."reason", 2000) OR
public.text_values_exceed_bound(NEW."review_reasons", OLD."review_reasons", 2000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000) OR
public.text_value_exceeds_bound(NEW."source_hash", OLD."source_hash", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000) OR
public.text_value_exceeds_bound(NEW."voice_profile_md", OLD."voice_profile_md", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."web_form" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."allowed_origins", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."consent_policy_version", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."consent_wording", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."form_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."lead_origin", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."name", NULL, 120) OR
public.text_value_exceeds_bound(NEW."privacy_notice", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."privacy_notice_url", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."redirect_url", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."secret_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."sender", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."success_message", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."web_form" FOR EACH ROW
WHEN (public.text_values_exceed_bound(NEW."allowed_origins", OLD."allowed_origins", 20000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."consent_policy_version", OLD."consent_policy_version", 20000) OR
public.text_value_exceeds_bound(NEW."consent_wording", OLD."consent_wording", 20000) OR
public.text_value_exceeds_bound(NEW."form_key", OLD."form_key", 20000) OR
public.text_value_exceeds_bound(NEW."lead_origin", OLD."lead_origin", 20000) OR
public.text_value_exceeds_bound(NEW."name", OLD."name", 120) OR
public.text_value_exceeds_bound(NEW."privacy_notice", OLD."privacy_notice", 20000) OR
public.text_value_exceeds_bound(NEW."privacy_notice_url", OLD."privacy_notice_url", 20000) OR
public.text_value_exceeds_bound(NEW."redirect_url", OLD."redirect_url", 20000) OR
public.text_value_exceeds_bound(NEW."secret_ref", OLD."secret_ref", 20000) OR
public.text_value_exceeds_bound(NEW."sender", OLD."sender", 20000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000) OR
public.text_value_exceeds_bound(NEW."success_message", OLD."success_message", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."webhook_delivery" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."content_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."event_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."last_error", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."payload", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."webhook_delivery" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."content_type", OLD."content_type", 20000) OR
public.text_value_exceeds_bound(NEW."entity_type", OLD."entity_type", 20000) OR
public.text_value_exceeds_bound(NEW."event_type", OLD."event_type", 20000) OR
public.text_value_exceeds_bound(NEW."last_error", OLD."last_error", 20000) OR
public.text_value_exceeds_bound(NEW."payload", OLD."payload", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."webhook_subscription" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."body_content_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."body_template", NULL, 20000) OR
public.text_values_exceed_bound(NEW."event_types", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."signing_secret_ref", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."target_url", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."webhook_subscription" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."body_content_type", OLD."body_content_type", 20000) OR
public.text_value_exceeds_bound(NEW."body_template", OLD."body_template", 20000) OR
public.text_values_exceed_bound(NEW."event_types", OLD."event_types", 20000) OR
public.text_value_exceeds_bound(NEW."signing_secret_ref", OLD."signing_secret_ref", 20000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000) OR
public.text_value_exceeds_bound(NEW."target_url", OLD."target_url", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."weekly_plan" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."capacity_note", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."risks", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."weekly_plan" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."capacity_note", OLD."capacity_note", 2000) OR
public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."risks", OLD."risks", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."weekly_plan_commitment" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."help_requested", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."linked_record_type", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."manager_response", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."state", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."weekly_plan_commitment" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."captured_by", OLD."captured_by", 20000) OR
public.text_value_exceeds_bound(NEW."help_requested", OLD."help_requested", 20000) OR
public.text_value_exceeds_bound(NEW."label", OLD."label", 120) OR
public.text_value_exceeds_bound(NEW."linked_record_type", OLD."linked_record_type", 20000) OR
public.text_value_exceeds_bound(NEW."manager_response", OLD."manager_response", 20000) OR
public.text_value_exceeds_bound(NEW."state", OLD."state", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."weekly_review" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."learnings_state", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."mail_error", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."narrative", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."weekly_review" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."learnings_state", OLD."learnings_state", 20000) OR
public.text_value_exceeds_bound(NEW."mail_error", OLD."mail_error", 20000) OR
public.text_value_exceeds_bound(NEW."narrative", OLD."narrative", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."weekly_review_deal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."deal_label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."outcome", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."to_stage_label", NULL, 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."weekly_review_deal" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."deal_label", OLD."deal_label", 120) OR
public.text_value_exceeds_bound(NEW."outcome", OLD."outcome", 20000) OR
public.text_value_exceeds_bound(NEW."to_stage_label", OLD."to_stage_label", 120)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."weekly_review_driver" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."bar", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."deal_label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."period_kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."weekly_review_driver" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."bar", OLD."bar", 20000) OR
public.text_value_exceeds_bound(NEW."deal_label", OLD."deal_label", 120) OR
public.text_value_exceeds_bound(NEW."period_kind", OLD."period_kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."weekly_review_learning" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."text", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."weekly_review_learning" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."kind", OLD."kind", 20000) OR
public.text_value_exceeds_bound(NEW."text", OLD."text", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."weekly_review_learning_citation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."label", NULL, 120) OR
public.text_value_exceeds_bound(NEW."subject_type", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."weekly_review_learning_citation" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."label", OLD."label", 120) OR
public.text_value_exceeds_bound(NEW."subject_type", OLD."subject_type", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."weekly_review_movement" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."bar", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."period_kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."weekly_review_movement" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."bar", OLD."bar", 20000) OR
public.text_value_exceeds_bound(NEW."period_kind", OLD."period_kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."weekly_review_outlook" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."forward_measure", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."period_kind", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."weekly_review_outlook" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."forward_measure", OLD."forward_measure", 20000) OR
public.text_value_exceeds_bound(NEW."period_kind", OLD."period_kind", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."withdrawal_credential" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."address", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."revoked_reason", NULL, 2000) OR
public.text_value_exceeds_bound(NEW."scope", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."token_hash", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."withdrawal_credential" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."address", OLD."address", 20000) OR
public.text_value_exceeds_bound(NEW."revoked_reason", OLD."revoked_reason", 2000) OR
public.text_value_exceeds_bound(NEW."scope", OLD."scope", 20000) OR
public.text_value_exceeds_bound(NEW."token_hash", OLD."token_hash", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."workflow_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."error", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."handler", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."idempotency_key", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."status", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."workflow_run" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."error", OLD."error", 20000) OR
public.text_value_exceeds_bound(NEW."handler", OLD."handler", 20000) OR
public.text_value_exceeds_bound(NEW."idempotency_key", OLD."idempotency_key", 20000) OR
public.text_value_exceeds_bound(NEW."status", OLD."status", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."worklist_done" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."row_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."set_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."worklist_done" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."row_id", OLD."row_id", 20000) OR
public.text_value_exceeds_bound(NEW."set_by", OLD."set_by", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."worklist_pin" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."row_id", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."set_by", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."worklist_pin" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."row_id", OLD."row_id", 20000) OR
public.text_value_exceeds_bound(NEW."set_by", OLD."set_by", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."worklist_snapshot" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."params_fingerprint", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."worklist_snapshot" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."params_fingerprint", OLD."params_fingerprint", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_insert" BEFORE INSERT ON "public"."workspace_email_domain" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."domain", NULL, 20000) OR
public.text_value_exceeds_bound(NEW."source", NULL, 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
CREATE TRIGGER "text_bound_update" BEFORE UPDATE ON "public"."workspace_email_domain" FOR EACH ROW
WHEN (public.text_value_exceeds_bound(NEW."domain", OLD."domain", 20000) OR
public.text_value_exceeds_bound(NEW."source", OLD."source", 20000)) EXECUTE FUNCTION public.refuse_oversized_text();
