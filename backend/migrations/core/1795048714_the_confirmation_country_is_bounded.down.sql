-- pgmigrate:no-transaction
SET lock_timeout = '2s';
DROP TRIGGER IF EXISTS "text_bound_insert" ON "public"."lawful_basis_confirmation";
DROP TRIGGER IF EXISTS "text_bound_update" ON "public"."lawful_basis_confirmation";
RESET lock_timeout;
