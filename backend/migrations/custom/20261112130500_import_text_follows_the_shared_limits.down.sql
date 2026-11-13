SET LOCAL lock_timeout = '2s';
DROP TRIGGER "text_bound_insert" ON "public"."import_record_map";
DROP TRIGGER "text_bound_update" ON "public"."import_record_map";
DROP TRIGGER "text_bound_insert" ON "public"."import_run";
DROP TRIGGER "text_bound_update" ON "public"."import_run";
