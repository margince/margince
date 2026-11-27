-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The column comes back holding the one value the import surface wrote, with
-- the text guards that bounded it.
SET LOCAL lock_timeout = '2s';

ALTER TABLE import_run ADD COLUMN source text NOT NULL DEFAULT 'import_api';
ALTER TABLE import_run ALTER COLUMN source DROP DEFAULT;

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."import_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"approved_by":20000,"captured_by":20000,"connector":20000,"draft_step":20000,"error":20000,"job_status":20000,"phase":20000,"phase_object":20000,"source":20000,"source_ref":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"approved_by":20000,"captured_by":20000,"connector":20000,"draft_step":20000,"error":20000,"job_status":20000,"phase":20000,"phase_object":20000,"source":20000,"source_ref":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."import_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"approved_by":20000,"captured_by":20000,"connector":20000,"draft_step":20000,"error":20000,"job_status":20000,"phase":20000,"phase_object":20000,"source":20000,"source_ref":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"approved_by":20000,"captured_by":20000,"connector":20000,"draft_step":20000,"error":20000,"job_status":20000,"phase":20000,"phase_object":20000,"source":20000,"source_ref":20000,"status":20000}');
