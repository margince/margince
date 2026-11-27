-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- No statement reads import_run.source: the served run names its surface from
-- a constant. The text guards are set again without the column.
SET LOCAL lock_timeout = '2s';

ALTER TABLE import_run DROP COLUMN source;

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."import_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"approved_by":20000,"captured_by":20000,"connector":20000,"draft_step":20000,"error":20000,"job_status":20000,"phase":20000,"phase_object":20000,"source_ref":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"approved_by":20000,"captured_by":20000,"connector":20000,"draft_step":20000,"error":20000,"job_status":20000,"phase":20000,"phase_object":20000,"source_ref":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."import_run" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"approved_by":20000,"captured_by":20000,"connector":20000,"draft_step":20000,"error":20000,"job_status":20000,"phase":20000,"phase_object":20000,"source_ref":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"approved_by":20000,"captured_by":20000,"connector":20000,"draft_step":20000,"error":20000,"job_status":20000,"phase":20000,"phase_object":20000,"source_ref":20000,"status":20000}');
