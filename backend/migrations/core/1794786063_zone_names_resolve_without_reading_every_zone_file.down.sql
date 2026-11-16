CREATE OR REPLACE FUNCTION public.valid_intent_time(wall_clock timestamp without time zone, zone_name text)
RETURNS boolean LANGUAGE sql STABLE SET search_path = pg_catalog AS $$
 SELECT CASE
  WHEN wall_clock IS NULL AND zone_name IS NULL THEN true
  WHEN wall_clock IS NOT NULL AND zone_name IS NOT NULL
   AND isfinite(wall_clock) AND extract(year FROM wall_clock) BETWEEN 1 AND 9999
   AND zone_name <> 'Local' AND btrim(zone_name) = zone_name
   AND EXISTS (SELECT 1 FROM pg_timezone_names WHERE name = zone_name)
  THEN extract(year FROM public.resolve_intent_time(wall_clock, zone_name)) BETWEEN 1 AND 9999
  ELSE false
 END
$$;

DROP FUNCTION public.named_time_zone(text);
