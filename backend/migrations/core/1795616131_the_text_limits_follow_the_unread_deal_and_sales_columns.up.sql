-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_sales_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"channel_provider":20000,"kind":20000,"thread_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"channel_provider":20000,"kind":20000,"thread_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_sales_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"channel_provider":20000,"kind":20000,"thread_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"channel_provider":20000,"kind":20000,"thread_key":20000}');
DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."deal_document_hide";
DROP TRIGGER IF EXISTS "text_bound_update" ON "public"."deal_document_hide";
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_room_invitation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_room_invitation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_room_session" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_room_session" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."sales_initiative" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"description":20000,"name":120,"objective_metric":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"description":20000,"name":120,"objective_metric":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."sales_initiative" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"description":20000,"name":120,"objective_metric":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"description":20000,"name":120,"objective_metric":20000,"status":20000}');
DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."sales_initiative_baseline";
DROP TRIGGER IF EXISTS "text_bound_update" ON "public"."sales_initiative_baseline";
RESET lock_timeout;
