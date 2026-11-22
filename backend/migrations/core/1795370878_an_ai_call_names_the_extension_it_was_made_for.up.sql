-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- An ai_call row names the extension it was made for, as "<unit>/<task>", so an
-- operator can list every model call an installed extension caused and what each
-- one was for. Empty for the product's own calls.
--
-- Bounded to one line like the other short names, and recreated with the text
-- limit triggers so the new column is held to it.
SET LOCAL lock_timeout = '5s';

ALTER TABLE ai_call ADD COLUMN extension text NOT NULL DEFAULT '';

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."ai_call" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"attempt_reason":2000,"config_hash":20000,"context_fingerprint":20000,"context_scopes":20000,"decision_choice":20000,"error_sentinel":20000,"extension":500,"finish_reason":2000,"kind":20000,"model_id":20000,"provider":20000,"request_fingerprint":20000,"schema_downgrade":20000,"secret_kinds":20000,"served_identity_source":20000,"served_model":20000,"served_provider":20000,"subject_type":20000,"task":20000,"tier":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"attempt_reason":2000,"config_hash":20000,"context_fingerprint":20000,"context_scopes":20000,"decision_choice":20000,"error_sentinel":20000,"extension":500,"finish_reason":2000,"kind":20000,"model_id":20000,"provider":20000,"request_fingerprint":20000,"schema_downgrade":20000,"secret_kinds":20000,"served_identity_source":20000,"served_model":20000,"served_provider":20000,"subject_type":20000,"task":20000,"tier":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."ai_call" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"attempt_reason":2000,"config_hash":20000,"context_fingerprint":20000,"context_scopes":20000,"decision_choice":20000,"error_sentinel":20000,"extension":500,"finish_reason":2000,"kind":20000,"model_id":20000,"provider":20000,"request_fingerprint":20000,"schema_downgrade":20000,"secret_kinds":20000,"served_identity_source":20000,"served_model":20000,"served_provider":20000,"subject_type":20000,"task":20000,"tier":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"attempt_reason":2000,"config_hash":20000,"context_fingerprint":20000,"context_scopes":20000,"decision_choice":20000,"error_sentinel":20000,"extension":500,"finish_reason":2000,"kind":20000,"model_id":20000,"provider":20000,"request_fingerprint":20000,"schema_downgrade":20000,"secret_kinds":20000,"served_identity_source":20000,"served_model":20000,"served_provider":20000,"subject_type":20000,"task":20000,"tier":20000}');
