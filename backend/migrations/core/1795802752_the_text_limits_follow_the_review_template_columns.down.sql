-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."activity_review_template" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"key":20000,"label":120,"outcome":20000,"shipped_label":120,"shipped_label_key":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"key":20000,"label":120,"outcome":20000,"shipped_label":120,"shipped_label_key":20000,"source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."activity_review_template" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"key":20000,"label":120,"outcome":20000,"shipped_label":120,"shipped_label_key":20000,"source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"key":20000,"label":120,"outcome":20000,"shipped_label":120,"shipped_label_key":20000,"source":20000}');
RESET lock_timeout;
