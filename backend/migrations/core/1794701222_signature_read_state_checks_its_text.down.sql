-- pgmigrate:no-transaction
SET lock_timeout = '2s';
DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."contact_signature_enrich_state";
DROP TRIGGER IF EXISTS "text_bound_update" ON "public"."contact_signature_enrich_state";
RESET lock_timeout;
