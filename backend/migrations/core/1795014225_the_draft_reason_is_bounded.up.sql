-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."mail_draft" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"anchor_type":20000,"bcc_addresses":20000,"cc_addresses":20000,"communication_context":20000,"override_explanation":2000,"scheduled_tz":20000,"subject":20000,"to_addresses":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"anchor_type":20000,"bcc_addresses":20000,"cc_addresses":20000,"communication_context":20000,"override_explanation":2000,"scheduled_tz":20000,"subject":20000,"to_addresses":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."mail_draft" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"anchor_type":20000,"bcc_addresses":20000,"cc_addresses":20000,"communication_context":20000,"override_explanation":2000,"scheduled_tz":20000,"subject":20000,"to_addresses":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"anchor_type":20000,"bcc_addresses":20000,"cc_addresses":20000,"communication_context":20000,"override_explanation":2000,"scheduled_tz":20000,"subject":20000,"to_addresses":20000}');
RESET lock_timeout;
