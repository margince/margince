CREATE OR REPLACE FUNCTION public.named_time_zone(zone_name text)
RETURNS boolean LANGUAGE plpgsql STABLE STRICT SET search_path = pg_catalog AS $$
DECLARE
 canonical text;
BEGIN
 IF zone_name ~ '[0-9]' THEN
  RETURN EXISTS (SELECT 1 FROM pg_timezone_names WHERE name = zone_name);
 END IF;
 IF zone_name ~* '^interval' THEN
  RETURN false;
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
