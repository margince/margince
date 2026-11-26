SET LOCAL lock_timeout = '2s';
DO $bounds$
DECLARE target record;
BEGIN
 FOR target IN
  SELECT c.relname, t.tgname
  FROM pg_catalog.pg_trigger t
  JOIN pg_catalog.pg_class c ON c.oid = t.tgrelid
  JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname = 'public' AND t.tgname IN ('custom_text_bound_insert', 'custom_text_bound_update')
 LOOP
  EXECUTE format('DROP TRIGGER %I ON public.%I', target.tgname, target.relname);
 END LOOP;
END
$bounds$;
