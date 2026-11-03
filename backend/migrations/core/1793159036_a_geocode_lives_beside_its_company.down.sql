SET LOCAL lock_timeout = '3s';
CREATE OR REPLACE FUNCTION company_geocode_goes_stale() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  -- Only when the coordinates exist and the writer did not speak for them:
  -- stamping 'stale' on a row that was never resolved would say the
  -- coordinates are out of date rather than absent, and overriding a writer
  -- that set the status deliberately would undo the worker's own write.
  IF NEW.geocode_status IS DISTINCT FROM OLD.geocode_status THEN
    RETURN NEW;
  END IF;
  IF OLD.geocode_status IS NULL THEN
    RETURN NEW;
  END IF;
  NEW.geocode_status := 'stale';
  RETURN NEW;
END;
$$;
CREATE TRIGGER trg_company_geocode_stale
    BEFORE UPDATE OF address_line1, address_line2, address_city, address_region, address_postal_code, address_country
    ON company FOR EACH ROW
    WHEN (OLD.address_line1 IS DISTINCT FROM NEW.address_line1
       OR OLD.address_line2 IS DISTINCT FROM NEW.address_line2
       OR OLD.address_city IS DISTINCT FROM NEW.address_city
       OR OLD.address_region IS DISTINCT FROM NEW.address_region
       OR OLD.address_postal_code IS DISTINCT FROM NEW.address_postal_code
       OR OLD.address_country IS DISTINCT FROM NEW.address_country)
    EXECUTE FUNCTION company_geocode_goes_stale();
DROP TRIGGER IF EXISTS trg_company_geocode_state_stale ON company;
DROP FUNCTION IF EXISTS company_geocode_state_goes_stale();
-- The points answered since the upgrade go back onto the company row, which
-- the restored readers read.
UPDATE company c
   SET geocode_lat = g.lat, geocode_lon = g.lon, geocode_status = g.status,
       geocode_provider = g.provider, geocode_input_hash = g.input_hash, geocoded_at = g.geocoded_at
  FROM company_geocode_state g
 WHERE g.company_id = c.id AND g.status IS NOT NULL;
ALTER TABLE company_geocode_state
    DROP CONSTRAINT IF EXISTS company_geocode_state_status_check,
    DROP COLUMN IF EXISTS geocoded_at,
    DROP COLUMN IF EXISTS input_hash,
    DROP COLUMN IF EXISTS provider,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS lon,
    DROP COLUMN IF EXISTS lat;
