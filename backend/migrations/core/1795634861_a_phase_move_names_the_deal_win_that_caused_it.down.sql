-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Dropping the columns loses which phase moves a deal win caused; those rows then
-- read as moves without a reason.
SET LOCAL lock_timeout = '5s';

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."project_phase_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"changed_by":20000,"from_phase":20000,"reason":2000,"to_phase":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"changed_by":20000,"from_phase":20000,"reason":2000,"to_phase":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."project_phase_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"changed_by":20000,"from_phase":20000,"reason":2000,"to_phase":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"changed_by":20000,"from_phase":20000,"reason":2000,"to_phase":20000}');

ALTER TABLE project_phase_history
    DROP CONSTRAINT IF EXISTS project_phase_history_cause_pairs,
    DROP CONSTRAINT IF EXISTS project_phase_history_cause_kind_check,
    DROP COLUMN IF EXISTS cause_deal_id,
    DROP COLUMN IF EXISTS cause_kind;
