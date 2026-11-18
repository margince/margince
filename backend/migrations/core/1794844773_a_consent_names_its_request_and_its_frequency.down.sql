SET LOCAL lock_timeout = '3s';

ALTER TABLE consent_event DROP COLUMN subject_request_id;
ALTER TABLE confirm_token DROP COLUMN subject_request_id;
ALTER TABLE confirm_token DROP COLUMN asks_news;
ALTER TABLE contact_acquisition_evidence DROP CONSTRAINT contact_acquisition_evidence_stated_source_check;
ALTER TABLE contact_acquisition_evidence DROP COLUMN stated_source;
ALTER TABLE consent_purpose DROP CONSTRAINT consent_purpose_frequency_check;
ALTER TABLE consent_purpose DROP COLUMN frequency;

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."consent_purpose" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"class":20000,"key":20000,"label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"class":20000,"key":20000,"label":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."consent_purpose" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"class":20000,"key":20000,"label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"class":20000,"key":20000,"label":120}');

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_acquisition_evidence" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"kind":20000,"note":2000,"purpose_claimed":20000,"recorded_for_addresses":20000,"request_retracted_by":20000,"source_entity_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"kind":20000,"note":2000,"purpose_claimed":20000,"recorded_for_addresses":20000,"request_retracted_by":20000,"source_entity_type":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_acquisition_evidence" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"kind":20000,"note":2000,"purpose_claimed":20000,"recorded_for_addresses":20000,"request_retracted_by":20000,"source_entity_type":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"kind":20000,"note":2000,"purpose_claimed":20000,"recorded_for_addresses":20000,"request_retracted_by":20000,"source_entity_type":20000}');
