-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."finance_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"credential_ref":20000,"last_error_code":20000,"provider":20000,"source":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"credential_ref":20000,"last_error_code":20000,"provider":20000,"source":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."finance_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"credential_ref":20000,"last_error_code":20000,"provider":20000,"source":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"credential_ref":20000,"last_error_code":20000,"provider":20000,"source":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."forecast_contribution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"exclusion_reason":2000,"probability_source":20000,"qualification":20000,"risk_signals":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"exclusion_reason":2000,"probability_source":20000,"qualification":20000,"risk_signals":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."forecast_contribution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"exclusion_reason":2000,"probability_source":20000,"qualification":20000,"risk_signals":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"exclusion_reason":2000,"probability_source":20000,"qualification":20000,"risk_signals":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."workflow_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"handler":20000,"idempotency_key":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"handler":20000,"idempotency_key":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."workflow_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"handler":20000,"idempotency_key":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"handler":20000,"idempotency_key":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."assurance_task_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"subject_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"subject_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."assurance_task_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"subject_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"subject_kind":20000}');
RESET lock_timeout;
