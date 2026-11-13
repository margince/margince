SET LOCAL lock_timeout = '2s';
CREATE OR REPLACE FUNCTION public.text_value_exceeds_bound(new_value text, old_value text, character_limit integer)
RETURNS boolean LANGUAGE sql IMMUTABLE PARALLEL SAFE SET search_path = pg_catalog AS $$
SELECT new_value IS NOT NULL AND new_value COLLATE "C" IS DISTINCT FROM old_value COLLATE "C"
 AND char_length(new_value) > character_limit
$$;
CREATE OR REPLACE FUNCTION public.text_values_exceed_bound(new_values text[], old_values text[], character_limit integer)
RETURNS boolean LANGUAGE sql IMMUTABLE PARALLEL SAFE SET search_path = pg_catalog AS $$
SELECT EXISTS (SELECT 1 FROM unnest(new_values) AS item(value)
 WHERE char_length(value) > character_limit
 AND NOT EXISTS (SELECT 1 FROM unnest(old_values) AS previous(value)
                 WHERE previous.value COLLATE "C" = item.value COLLATE "C"))
$$;
CREATE OR REPLACE FUNCTION public.text_row_exceeds_bounds(new_row jsonb, old_row jsonb, bounds jsonb)
RETURNS boolean LANGUAGE sql IMMUTABLE PARALLEL SAFE SET search_path = pg_catalog AS $$
SELECT EXISTS (SELECT 1 FROM jsonb_each_text(bounds) AS bound(key, maximum)
 WHERE CASE jsonb_typeof(new_row -> bound.key)
  WHEN 'string' THEN public.text_value_exceeds_bound(new_row ->> bound.key, old_row ->> bound.key, bound.maximum::integer)
  WHEN 'array' THEN public.text_values_exceed_bound(
   ARRAY(SELECT jsonb_array_elements_text(new_row -> bound.key)),
   ARRAY(SELECT jsonb_array_elements_text(NULLIF(old_row -> bound.key, 'null'::jsonb))), bound.maximum::integer)
  ELSE false END)
$$;
CREATE OR REPLACE FUNCTION public.refuse_oversized_text() RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
 bound record;
 inserted jsonb := to_jsonb(NEW);
 previous jsonb;
BEGIN
 IF TG_OP = 'UPDATE' THEN previous := to_jsonb(OLD); END IF;
 FOR bound IN SELECT key, value FROM jsonb_each(TG_ARGV[0]::jsonb)
 LOOP
  IF public.text_row_exceeds_bounds(inserted, previous, jsonb_build_object(bound.key, bound.value)) THEN
   RAISE EXCEPTION 'text exceeds its character limit' USING ERRCODE = '22001',
    CONSTRAINT = 'text_character_bound', COLUMN = bound.key, DETAIL = bound.value::text;
  END IF;
 END LOOP;
 RAISE EXCEPTION 'text exceeds its character limit' USING ERRCODE = '22001';
END
$$;
