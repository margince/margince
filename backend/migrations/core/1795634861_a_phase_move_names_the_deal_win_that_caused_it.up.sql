-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A phase move that no one chose by hand names what caused it. cause_kind is a
-- closed code a client words in the reader's language; deal_won carries the deal
-- whose win moved the project into delivering. A move a person made has neither.
--
-- No foreign key on cause_deal_id: deal is owned by another module, and erasing a
-- deal must not be blocked by a history row. A reader resolves the id under its
-- own deal scope, so a gone deal reads as unnamed. Existing rows stay NULL.
--
-- The constraints are added NOT VALID; the next migration validates them. The
-- text limit triggers are recreated so the new column is held to its limit.
SET LOCAL lock_timeout = '5s';

ALTER TABLE project_phase_history
    ADD COLUMN cause_kind text,
    ADD COLUMN cause_deal_id uuid,
    ADD CONSTRAINT project_phase_history_cause_kind_check CHECK (
        cause_kind = ANY (ARRAY['deal_won'])) NOT VALID,
    ADD CONSTRAINT project_phase_history_cause_pairs CHECK (
        (cause_kind IS NULL) = (cause_deal_id IS NULL)) NOT VALID;

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."project_phase_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"cause_kind":120,"changed_by":20000,"from_phase":20000,"reason":2000,"to_phase":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"cause_kind":120,"changed_by":20000,"from_phase":20000,"reason":2000,"to_phase":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."project_phase_history" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"cause_kind":120,"changed_by":20000,"from_phase":20000,"reason":2000,"to_phase":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"cause_kind":120,"changed_by":20000,"from_phase":20000,"reason":2000,"to_phase":20000}');
