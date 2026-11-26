-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."field_provenance" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"evidence_ref":20000,"field_name":120,"object_type":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"evidence_ref":20000,"field_name":120,"object_type":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."field_provenance" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"evidence_ref":20000,"field_name":120,"object_type":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"evidence_ref":20000,"field_name":120,"object_type":20000,"source":20000}');
RESET lock_timeout;
