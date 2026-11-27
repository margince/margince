-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."assurance_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"readiness":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"readiness":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."assurance_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"readiness":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"readiness":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."finance_invoice" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"external_id":20000,"number":20000,"source":20000,"status":20000,"sync_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"external_id":20000,"number":20000,"source":20000,"status":20000,"sync_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."finance_invoice" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"external_id":20000,"number":20000,"source":20000,"status":20000,"sync_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"external_id":20000,"number":20000,"source":20000,"status":20000,"sync_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_provider_claim" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"claim_key":20000,"employment_processing_error":20000,"provider":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"claim_key":20000,"employment_processing_error":20000,"provider":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_provider_claim" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"claim_key":20000,"employment_processing_error":20000,"provider":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"claim_key":20000,"employment_processing_error":20000,"provider":20000,"source":20000}');
RESET lock_timeout;
