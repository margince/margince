-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."graph_contact_edge" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"last_precision":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"last_precision":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."graph_contact_edge" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"last_precision":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"last_precision":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."graph_interaction_edge" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"last_precision":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"last_precision":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."graph_interaction_edge" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"last_precision":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"last_precision":20000}');
RESET lock_timeout;
