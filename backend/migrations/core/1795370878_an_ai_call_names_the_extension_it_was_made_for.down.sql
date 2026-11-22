-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Dropping the column loses which extension each call was made for, which is the
-- point of the additive rule: this direction exists for a rollback that has not
-- yet run in production.
SET LOCAL lock_timeout = '5s';

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."ai_call" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"attempt_reason":2000,"config_hash":20000,"context_fingerprint":20000,"context_scopes":20000,"decision_choice":20000,"error_sentinel":20000,"finish_reason":2000,"kind":20000,"model_id":20000,"provider":20000,"request_fingerprint":20000,"schema_downgrade":20000,"secret_kinds":20000,"served_identity_source":20000,"served_model":20000,"served_provider":20000,"subject_type":20000,"task":20000,"tier":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"attempt_reason":2000,"config_hash":20000,"context_fingerprint":20000,"context_scopes":20000,"decision_choice":20000,"error_sentinel":20000,"finish_reason":2000,"kind":20000,"model_id":20000,"provider":20000,"request_fingerprint":20000,"schema_downgrade":20000,"secret_kinds":20000,"served_identity_source":20000,"served_model":20000,"served_provider":20000,"subject_type":20000,"task":20000,"tier":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."ai_call" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"attempt_reason":2000,"config_hash":20000,"context_fingerprint":20000,"context_scopes":20000,"decision_choice":20000,"error_sentinel":20000,"finish_reason":2000,"kind":20000,"model_id":20000,"provider":20000,"request_fingerprint":20000,"schema_downgrade":20000,"secret_kinds":20000,"served_identity_source":20000,"served_model":20000,"served_provider":20000,"subject_type":20000,"task":20000,"tier":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"attempt_reason":2000,"config_hash":20000,"context_fingerprint":20000,"context_scopes":20000,"decision_choice":20000,"error_sentinel":20000,"finish_reason":2000,"kind":20000,"model_id":20000,"provider":20000,"request_fingerprint":20000,"schema_downgrade":20000,"secret_kinds":20000,"served_identity_source":20000,"served_model":20000,"served_provider":20000,"subject_type":20000,"task":20000,"tier":20000}');

ALTER TABLE ai_call DROP COLUMN IF EXISTS extension;
