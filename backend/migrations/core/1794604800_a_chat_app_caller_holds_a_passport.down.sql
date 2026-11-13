-- Reverses the chat app caller's passport binding.
SET LOCAL lock_timeout = '5s';

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."passport" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"label":120,"scopes":20000,"token_hash":20000,"work_areas":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"label":120,"scopes":20000,"token_hash":20000,"work_areas":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."passport" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"label":120,"scopes":20000,"token_hash":20000,"work_areas":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"label":120,"scopes":20000,"token_hash":20000,"work_areas":20000}');

DROP INDEX IF EXISTS idx_passport_app_caller;
ALTER TABLE passport DROP CONSTRAINT IF EXISTS passport_app_caller_pair;
ALTER TABLE passport DROP COLUMN IF EXISTS app_subject;
ALTER TABLE passport DROP COLUMN IF EXISTS app_issuer;
