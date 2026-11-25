-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."channel_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"address":500,"channel_id":20000,"channel_label":120,"credential_ref":20000,"events_credential_ref":20000,"provider":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address":500,"channel_id":20000,"channel_label":120,"credential_ref":20000,"events_credential_ref":20000,"provider":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."channel_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"address":500,"channel_id":20000,"channel_label":120,"credential_ref":20000,"events_credential_ref":20000,"provider":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address":500,"channel_id":20000,"channel_label":120,"credential_ref":20000,"events_credential_ref":20000,"provider":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_channel_identity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"channel_user_id":20000,"membership_bot_id":20000,"provider":20000,"source":20000,"username":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"channel_user_id":20000,"membership_bot_id":20000,"provider":20000,"source":20000,"username":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_channel_identity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"channel_user_id":20000,"membership_bot_id":20000,"provider":20000,"source":20000,"username":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"channel_user_id":20000,"membership_bot_id":20000,"provider":20000,"source":20000,"username":20000}');
RESET lock_timeout;
