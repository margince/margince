-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Why a task is on its reader's list, cached per reader like the record
-- briefs. A deleted task or reader takes its explanations with it.
SET LOCAL lock_timeout = '3s';

CREATE TABLE task_explanation (
    user_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    fingerprint text NOT NULL,
    payload jsonb NOT NULL,
    generated_by text NOT NULL,
    generated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT task_explanation_pkey PRIMARY KEY (user_id, activity_id),
    CONSTRAINT task_explanation_generated_by_check CHECK (generated_by IN ('model', 'deterministic')),
    CONSTRAINT task_explanation_user_fkey FOREIGN KEY (user_id) REFERENCES app_user(id) ON DELETE CASCADE,
    CONSTRAINT task_explanation_activity_fkey FOREIGN KEY (activity_id) REFERENCES activity(id) ON DELETE CASCADE
);

COMMENT ON TABLE task_explanation IS 'Read-model cache of why a task is on a reader''s list, keyed per READER.';

CREATE INDEX task_explanation_activity_ix ON task_explanation (activity_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE task_explanation TO margince_app;

CREATE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."task_explanation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
CREATE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."task_explanation" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
