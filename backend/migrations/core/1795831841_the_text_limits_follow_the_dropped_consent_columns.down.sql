-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."consent_text_version" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"channel":20000,"content_hash":20000,"jurisdiction":20000,"key":20000,"locale":20000,"subject_line":20000,"version":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"channel":20000,"content_hash":20000,"jurisdiction":20000,"key":20000,"locale":20000,"subject_line":20000,"version":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."consent_text_version" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"channel":20000,"content_hash":20000,"jurisdiction":20000,"key":20000,"locale":20000,"subject_line":20000,"version":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"channel":20000,"content_hash":20000,"jurisdiction":20000,"key":20000,"locale":20000,"subject_line":20000,"version":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_acquisition_evidence" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"kind":20000,"note":2000,"purpose_claimed":20000,"recorded_for_addresses":20000,"request_retracted_by":20000,"source_entity_type":20000,"stated_source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"kind":20000,"note":2000,"purpose_claimed":20000,"recorded_for_addresses":20000,"request_retracted_by":20000,"source_entity_type":20000,"stated_source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_acquisition_evidence" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"kind":20000,"note":2000,"purpose_claimed":20000,"recorded_for_addresses":20000,"request_retracted_by":20000,"source_entity_type":20000,"stated_source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"kind":20000,"note":2000,"purpose_claimed":20000,"recorded_for_addresses":20000,"request_retracted_by":20000,"source_entity_type":20000,"stated_source":20000}');
RESET lock_timeout;
