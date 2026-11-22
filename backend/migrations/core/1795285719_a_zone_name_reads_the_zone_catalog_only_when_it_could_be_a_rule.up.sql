-- Postgres finds a zone file in any case and reports the file's own spelling.
-- A POSIX rule such as UTC+3 needs no file and loads under its upper-case
-- spelling. So a name Postgres hands back unchanged names a zone file, unless
-- it is upper case with a digit and could be a rule. Only those names read the
-- zone catalog, which opens every zone file on each call.
-- Postgres parses a value that starts with "interval" as an interval, which is
-- no zone name, and a failed parse there raises an error other than a refusal.
-- The setting is undone with its subtransaction.
CREATE OR REPLACE FUNCTION public.named_time_zone(zone_name text)
RETURNS boolean LANGUAGE plpgsql STABLE STRICT SET search_path = pg_catalog AS $$
DECLARE
 canonical text;
BEGIN
 IF zone_name ~* '^interval' THEN
  RETURN false;
 END IF;
 BEGIN
  PERFORM set_config('TimeZone', zone_name, true);
  canonical := current_setting('TimeZone');
  RAISE EXCEPTION USING ERRCODE = 'MZ001';
 EXCEPTION
  WHEN SQLSTATE 'MZ001' THEN NULL;
  WHEN invalid_parameter_value THEN RETURN false;
 END;
 IF canonical <> zone_name THEN
  RETURN false;
 END IF;
 IF zone_name ~ '[0-9]' AND zone_name = upper(zone_name) THEN
  RETURN EXISTS (SELECT 1 FROM pg_timezone_names WHERE name = zone_name);
 END IF;
 RETURN true;
END
$$;
