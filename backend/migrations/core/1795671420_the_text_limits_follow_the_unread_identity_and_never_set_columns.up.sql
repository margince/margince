-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."auth_token" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."auth_token" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"token_hash":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"token_hash":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."federated_identity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"provider":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"provider":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."federated_identity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"provider":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"provider":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."linkedin_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"company_name":120,"email":20000,"match_status":20000,"normalized_company":20000,"normalized_name":120,"position":20000,"profile_url":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"company_name":120,"email":20000,"match_status":20000,"normalized_company":20000,"normalized_name":120,"position":20000,"profile_url":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."linkedin_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"company_name":120,"email":20000,"match_status":20000,"normalized_company":20000,"normalized_name":120,"position":20000,"profile_url":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"company_name":120,"email":20000,"match_status":20000,"normalized_company":20000,"normalized_name":120,"position":20000,"profile_url":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."partner" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"activated_by":20000,"activation_basis":20000,"captured_by":20000,"cert_status":20000,"margin_tier":20000,"next_step":20000,"partner_fit_override_reason":2000,"partner_kinds":20000,"relationship_stage":20000,"served_segments":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"activated_by":20000,"activation_basis":20000,"captured_by":20000,"cert_status":20000,"margin_tier":20000,"next_step":20000,"partner_fit_override_reason":2000,"partner_kinds":20000,"relationship_stage":20000,"served_segments":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."partner" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"activated_by":20000,"activation_basis":20000,"captured_by":20000,"cert_status":20000,"margin_tier":20000,"next_step":20000,"partner_fit_override_reason":2000,"partner_kinds":20000,"relationship_stage":20000,"served_segments":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"activated_by":20000,"activation_basis":20000,"captured_by":20000,"cert_status":20000,"margin_tier":20000,"next_step":20000,"partner_fit_override_reason":2000,"partner_kinds":20000,"relationship_stage":20000,"served_segments":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."project_phase_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"cause_kind":120,"from_phase":20000,"reason":2000,"to_phase":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"cause_kind":120,"from_phase":20000,"reason":2000,"to_phase":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."project_phase_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"cause_kind":120,"from_phase":20000,"reason":2000,"to_phase":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"cause_kind":120,"from_phase":20000,"reason":2000,"to_phase":20000}');
RESET lock_timeout;
