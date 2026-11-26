-- pgmigrate:no-transaction
SET lock_timeout = '2s';
DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."graph_contact_edge";
DROP TRIGGER IF EXISTS "text_bound_update" ON "public"."graph_contact_edge";
DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."graph_interaction_edge";
DROP TRIGGER IF EXISTS "text_bound_update" ON "public"."graph_interaction_edge";
RESET lock_timeout;
