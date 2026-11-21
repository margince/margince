-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."finance_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"credential_ref":20000,"last_error_code":20000,"provider":20000,"source":20000,"status":20000,"sync_cursor":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"credential_ref":20000,"last_error_code":20000,"provider":20000,"source":20000,"status":20000,"sync_cursor":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."finance_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"credential_ref":20000,"last_error_code":20000,"provider":20000,"source":20000,"status":20000,"sync_cursor":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"credential_ref":20000,"last_error_code":20000,"provider":20000,"source":20000,"status":20000,"sync_cursor":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."forecast_contribution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"category":20000,"exclusion_reason":2000,"probability_source":20000,"qualification":20000,"qualified_policy_version":20000,"risk_signals":20000,"viability_disposition":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"category":20000,"exclusion_reason":2000,"probability_source":20000,"qualification":20000,"qualified_policy_version":20000,"risk_signals":20000,"viability_disposition":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."forecast_contribution" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"category":20000,"exclusion_reason":2000,"probability_source":20000,"qualification":20000,"qualified_policy_version":20000,"risk_signals":20000,"viability_disposition":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"category":20000,"exclusion_reason":2000,"probability_source":20000,"qualification":20000,"qualified_policy_version":20000,"risk_signals":20000,"viability_disposition":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."workflow_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"error":20000,"handler":20000,"idempotency_key":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"error":20000,"handler":20000,"idempotency_key":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."workflow_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"error":20000,"handler":20000,"idempotency_key":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"error":20000,"handler":20000,"idempotency_key":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."assurance_task_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"state":20000,"subject_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"state":20000,"subject_kind":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."assurance_task_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"state":20000,"subject_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"state":20000,"subject_kind":20000}');
RESET lock_timeout;
