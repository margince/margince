-- A zone name whose spelling has no digit cannot be a POSIX rule, so Postgres
-- accepts it as the session zone only when it loads a zone file under that
-- name: an abbreviation such as PST is refused there. Reading the canonical
-- spelling back keeps the match exact, since the file lookup ignores case.
-- A name with a digit (Etc/GMT+5, but also the POSIX rule UTC+3) keeps the
-- exact catalog lookup. The setting is undone with its subtransaction.
CREATE FUNCTION public.named_time_zone(zone_name text)
RETURNS boolean LANGUAGE plpgsql STABLE STRICT SET search_path = pg_catalog AS $$
DECLARE
 canonical text;
BEGIN
 IF zone_name ~ '[0-9]' THEN
  RETURN EXISTS (SELECT 1 FROM pg_timezone_names WHERE name = zone_name);
 END IF;
 BEGIN
  PERFORM set_config('TimeZone', zone_name, true);
  canonical := current_setting('TimeZone');
  RAISE EXCEPTION USING ERRCODE = 'MZ001';
 EXCEPTION
  WHEN SQLSTATE 'MZ001' THEN RETURN canonical = zone_name;
  WHEN invalid_parameter_value THEN RETURN false;
 END;
END
$$;

CREATE OR REPLACE FUNCTION public.valid_intent_time(wall_clock timestamp without time zone, zone_name text)
RETURNS boolean LANGUAGE sql STABLE SET search_path = pg_catalog AS $$
 SELECT CASE
  WHEN wall_clock IS NULL AND zone_name IS NULL THEN true
  WHEN wall_clock IS NOT NULL AND zone_name IS NOT NULL
   AND isfinite(wall_clock) AND extract(year FROM wall_clock) BETWEEN 1 AND 9999
   AND zone_name <> 'Local' AND btrim(zone_name) = zone_name
   AND public.named_time_zone(zone_name)
  THEN extract(year FROM public.resolve_intent_time(wall_clock, zone_name)) BETWEEN 1 AND 9999
  ELSE false
 END
$$;
