-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."chat_post_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"event_type":20000,"message_id":500}')) EXECUTE FUNCTION public.refuse_oversized_text('{"event_type":20000,"message_id":500}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."chat_post_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"event_type":20000,"message_id":500}')) EXECUTE FUNCTION public.refuse_oversized_text('{"event_type":20000,"message_id":500}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."chat_link" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"card_fingerprint":120,"card_message_id":500,"channel_name":500,"channel_slug":500,"provider":20000,"status":20000,"verbosity":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"card_fingerprint":120,"card_message_id":500,"channel_name":500,"channel_slug":500,"provider":20000,"status":20000,"verbosity":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."chat_link" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"card_fingerprint":120,"card_message_id":500,"channel_name":500,"channel_slug":500,"provider":20000,"status":20000,"verbosity":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"card_fingerprint":120,"card_message_id":500,"channel_name":500,"channel_slug":500,"provider":20000,"status":20000,"verbosity":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."report_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."report_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."report_edition_contribution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"context_id":20000,"contribution_key":20000,"metric":20000,"source_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"context_id":20000,"contribution_key":20000,"metric":20000,"source_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."report_edition_contribution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"context_id":20000,"contribution_key":20000,"metric":20000,"source_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"context_id":20000,"contribution_key":20000,"metric":20000,"source_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."runner_job" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"agent_spec":20000,"status":20000,"trigger_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"agent_spec":20000,"status":20000,"trigger_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."runner_job" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"agent_spec":20000,"status":20000,"trigger_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"agent_spec":20000,"status":20000,"trigger_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"account_label":120,"credential_ref":20000,"history_baseline_set_by":20000,"history_baseline_tz":20000,"mail_posture":20000,"provider":20000,"provider_scopes":20000,"status":20000,"watch_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"account_label":120,"credential_ref":20000,"history_baseline_set_by":20000,"history_baseline_tz":20000,"mail_posture":20000,"provider":20000,"provider_scopes":20000,"status":20000,"watch_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"account_label":120,"credential_ref":20000,"history_baseline_set_by":20000,"history_baseline_tz":20000,"mail_posture":20000,"provider":20000,"provider_scopes":20000,"status":20000,"watch_ref":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"account_label":120,"credential_ref":20000,"history_baseline_set_by":20000,"history_baseline_tz":20000,"mail_posture":20000,"provider":20000,"provider_scopes":20000,"status":20000,"watch_ref":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."capture_pending_counterparty" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"disposition_reason":2000,"domain":20000,"email":20000,"kind":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"disposition_reason":2000,"domain":20000,"email":20000,"kind":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."capture_pending_counterparty" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"disposition_reason":2000,"domain":20000,"email":20000,"kind":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"disposition_reason":2000,"domain":20000,"email":20000,"kind":20000,"status":20000}');
RESET lock_timeout;
