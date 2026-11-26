SET LOCAL lock_timeout = '2s';
DO $bounds$
DECLARE target record;
BEGIN
 FOR target IN
  SELECT f.object, jsonb_object_agg(f.column_name, 20000)::text AS limits
  FROM public.custom_field f
  JOIN pg_catalog.pg_namespace n ON n.nspname = 'public'
  JOIN pg_catalog.pg_class c ON c.relnamespace = n.oid AND c.relname = f.object
  JOIN pg_catalog.pg_attribute a ON a.attrelid = c.oid AND a.attname = f.column_name AND a.attnum > 0 AND NOT a.attisdropped
  WHERE f.type = 'text' AND a.atttypid = 'text'::regtype
  GROUP BY f.object
 LOOP
  EXECUTE format('CREATE OR REPLACE TRIGGER custom_text_bound_insert AFTER INSERT ON public.%I FOR EACH ROW WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, %L)) EXECUTE FUNCTION public.refuse_oversized_text(%L)', target.object, target.limits, target.limits);
  EXECUTE format('CREATE OR REPLACE TRIGGER custom_text_bound_update AFTER UPDATE ON public.%I FOR EACH ROW WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), %L)) EXECUTE FUNCTION public.refuse_oversized_text(%L)', target.object, target.limits, target.limits);
 END LOOP;
END
$bounds$;
