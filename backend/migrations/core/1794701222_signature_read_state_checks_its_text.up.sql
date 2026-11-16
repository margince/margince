-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_signature_enrich_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"last_outcome":120,"last_skip_reason":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"last_outcome":120,"last_skip_reason":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_signature_enrich_state" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"last_outcome":120,"last_skip_reason":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"last_outcome":120,"last_skip_reason":120}');
RESET lock_timeout;
