CREATE FUNCTION public.maintain_intent_times() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
 prior jsonb := CASE WHEN TG_OP = 'UPDATE' THEN to_jsonb(OLD) ELSE '{}'::jsonb END;
 requested jsonb := to_jsonb(NEW);
 slot integer;
 wall_name text;
 zone_name text;
 instant_name text;
 requested_wall timestamp without time zone;
 resolved record;
BEGIN
 FOR slot IN 0..TG_NARGS - 1 BY 3 LOOP
  wall_name := TG_ARGV[slot];
  zone_name := TG_ARGV[slot + 1];
  instant_name := TG_ARGV[slot + 2];
  requested_wall := (requested ->> wall_name)::timestamp without time zone;
  IF TG_OP = 'UPDATE' AND requested ->> wall_name IS NOT DISTINCT FROM prior ->> wall_name
   AND requested ->> zone_name IS NOT DISTINCT FROM prior ->> zone_name THEN
   requested_wall := NULL;
  END IF;
  SELECT * INTO resolved FROM public.update_intent_time(
   (prior ->> wall_name)::timestamp without time zone, prior ->> zone_name,
   (prior ->> instant_name)::timestamp with time zone,
   requested_wall, requested ->> zone_name, (requested ->> instant_name)::timestamp with time zone);
  NEW := jsonb_populate_record(NEW, jsonb_build_object(
   wall_name, resolved.wall_time, zone_name, resolved.timezone, instant_name, resolved.instant));
 END LOOP;
 RETURN NEW;
END
$$;
