-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."lawful_basis_confirmation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"country":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"country":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."lawful_basis_confirmation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"country":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"country":20000}');
RESET lock_timeout;
