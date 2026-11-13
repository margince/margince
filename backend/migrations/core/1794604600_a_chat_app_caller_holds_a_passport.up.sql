-- A person who calls Margince's MCP tools from an installed chat app (Dispact)
-- acts through a passport of their own. The app names the person by its own
-- issuer and subject, and the passport keeps that pair, so the next call finds
-- the same passport without a second mint. Revoking the passport stops the
-- app's calls for that person.
SET LOCAL lock_timeout = '5s';

ALTER TABLE passport ADD COLUMN app_issuer text;
ALTER TABLE passport ADD COLUMN app_subject text;
ALTER TABLE passport ADD CONSTRAINT passport_app_caller_pair
    CHECK ((app_issuer IS NULL) = (app_subject IS NULL));

CREATE INDEX idx_passport_app_caller ON passport (app_issuer, app_subject, created_at DESC)
    WHERE app_issuer IS NOT NULL;

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."passport" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"app_issuer":500,"app_subject":500,"label":120,"scopes":20000,"token_hash":20000,"work_areas":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"app_issuer":500,"app_subject":500,"label":120,"scopes":20000,"token_hash":20000,"work_areas":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."passport" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"app_issuer":500,"app_subject":500,"label":120,"scopes":20000,"token_hash":20000,"work_areas":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"app_issuer":500,"app_subject":500,"label":120,"scopes":20000,"token_hash":20000,"work_areas":20000}');
