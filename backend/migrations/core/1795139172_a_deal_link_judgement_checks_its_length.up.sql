-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_link" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"deal_relevance":20000,"entity_type":20000,"relevance_source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"deal_relevance":20000,"entity_type":20000,"relevance_source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_link" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"deal_relevance":20000,"entity_type":20000,"relevance_source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"deal_relevance":20000,"entity_type":20000,"relevance_source":20000}');
RESET lock_timeout;
