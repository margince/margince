CREATE FUNCTION public.resolve_intent_time(wall_clock timestamp without time zone, zone_name text)
RETURNS timestamp with time zone LANGUAGE plpgsql STABLE STRICT SET search_path = pg_catalog AS $$
DECLARE
 anchor timestamp with time zone := wall_clock AT TIME ZONE 'UTC';
 probe timestamp with time zone;
 zone_offset interval;
 candidate timestamp with time zone;
 candidates timestamp with time zone[] := ARRAY[]::timestamp with time zone[];
 first_match timestamp with time zone;
 low_instant timestamp with time zone;
 high_instant timestamp with time zone;
 middle timestamp with time zone;
BEGIN
 FOR probe IN SELECT generate_series(anchor - interval '48 hours', anchor + interval '48 hours', interval '24 hours') LOOP
  zone_offset := (probe AT TIME ZONE zone_name) - (probe AT TIME ZONE 'UTC');
  candidate := (wall_clock - zone_offset) AT TIME ZONE 'UTC';
  candidates := array_append(candidates, candidate);
  IF candidate AT TIME ZONE zone_name = wall_clock AND (first_match IS NULL OR candidate < first_match) THEN
   first_match := candidate;
  END IF;
 END LOOP;
 IF first_match IS NOT NULL THEN RETURN first_match; END IF;
 SELECT min(value), max(value) INTO low_instant, high_instant FROM unnest(candidates) AS value;
 WHILE high_instant - low_instant > interval '1 microsecond' LOOP
  middle := low_instant + (high_instant - low_instant) / 2;
  IF middle AT TIME ZONE zone_name < wall_clock THEN low_instant := middle;
  ELSE high_instant := middle;
  END IF;
 END LOOP;
 RETURN high_instant;
END
$$;

CREATE FUNCTION public.valid_intent_time(wall_clock timestamp without time zone, zone_name text)
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

CREATE FUNCTION public.update_intent_time(
 old_wall timestamp without time zone, old_zone text, old_instant timestamp with time zone,
 new_wall timestamp without time zone, new_zone text, new_instant timestamp with time zone)
RETURNS TABLE(wall_time timestamp without time zone, timezone text, instant timestamp with time zone)
LANGUAGE sql STABLE SET search_path = pg_catalog AS $$
 WITH kept AS MATERIALIZED (
  SELECT CASE
   WHEN new_wall IS NOT NULL THEN new_wall
   WHEN old_wall IS NOT NULL AND old_zone IS NOT NULL AND new_instant IS NOT NULL
    AND new_zone IS NOT DISTINCT FROM old_zone
    AND (new_instant = old_instant OR new_instant = public.resolve_intent_time(old_wall, old_zone))
   THEN old_wall
   ELSE NULL
  END AS wall_time
 )
 SELECT wall_time, CASE WHEN wall_time IS NOT NULL THEN new_zone ELSE NULL END,
  COALESCE(public.resolve_intent_time(wall_time, new_zone), new_instant)
 FROM kept
$$;
