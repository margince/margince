-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."automation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"key":20000,"name":120,"paused_reason":2000,"tier":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key":20000,"name":120,"paused_reason":2000,"tier":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."automation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"key":20000,"name":120,"paused_reason":2000,"tier":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"key":20000,"name":120,"paused_reason":2000,"tier":20000}');
DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."contact_signature_enrich_state";
DROP TRIGGER IF EXISTS "text_bound_update" ON "public"."contact_signature_enrich_state";
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."finance_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"last_error_code":20000,"provider":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"last_error_code":20000,"provider":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."finance_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"last_error_code":20000,"provider":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"last_error_code":20000,"provider":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."finance_customer_link" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"external_customer_id":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"external_customer_id":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."finance_customer_link" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"external_customer_id":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"external_customer_id":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."meeting_readiness" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"agenda_fingerprint":20000,"participants_fingerprint":20000,"revision":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"agenda_fingerprint":20000,"participants_fingerprint":20000,"revision":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."meeting_readiness" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"agenda_fingerprint":20000,"participants_fingerprint":20000,"revision":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"agenda_fingerprint":20000,"participants_fingerprint":20000,"revision":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_geocode_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"input_hash":20000,"provider":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"input_hash":20000,"provider":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_geocode_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"input_hash":20000,"provider":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"input_hash":20000,"provider":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."voice_learning_signal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"final_captured_by":20000,"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"final_captured_by":20000,"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."voice_learning_signal" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"final_captured_by":20000,"outcome":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"final_captured_by":20000,"outcome":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."voice_profile" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"active_source_hash":20000,"captured_by":20000,"personality_md":20000,"scope":20000,"source":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"active_source_hash":20000,"captured_by":20000,"personality_md":20000,"scope":20000,"source":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."voice_profile" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"active_source_hash":20000,"captured_by":20000,"personality_md":20000,"scope":20000,"source":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"active_source_hash":20000,"captured_by":20000,"personality_md":20000,"scope":20000,"source":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."worklist_done" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"row_id":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"row_id":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."worklist_done" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"row_id":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"row_id":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."worklist_pin" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"row_id":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"row_id":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."worklist_pin" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"row_id":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"row_id":20000,"source":20000}');
RESET lock_timeout;
