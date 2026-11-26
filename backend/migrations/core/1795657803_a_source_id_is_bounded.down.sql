-- pgmigrate:no-transaction
SET lock_timeout = '2s';
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."knowledge_document" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"checksum":20000,"content_type":20000,"filename":20000,"ingest_detail":20000,"ingest_status":20000,"managed_source":20000,"storage_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"checksum":20000,"content_type":20000,"filename":20000,"ingest_detail":20000,"ingest_status":20000,"managed_source":20000,"storage_key":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."knowledge_document" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"checksum":20000,"content_type":20000,"filename":20000,"ingest_detail":20000,"ingest_status":20000,"managed_source":20000,"storage_key":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"checksum":20000,"content_type":20000,"filename":20000,"ingest_detail":20000,"ingest_status":20000,"managed_source":20000,"storage_key":20000}');
RESET lock_timeout;
