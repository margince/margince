-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."communication_review" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"kind":20000,"reason_code":2000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000,"reason_code":2000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."communication_review" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"kind":20000,"reason_code":2000,"state":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"kind":20000,"reason_code":2000,"state":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."saved_view" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"name":120,"resource":20000,"shared_scope":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"name":120,"resource":20000,"shared_scope":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."saved_view" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"name":120,"resource":20000,"shared_scope":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"name":120,"resource":20000,"shared_scope":20000}');
RESET lock_timeout;
