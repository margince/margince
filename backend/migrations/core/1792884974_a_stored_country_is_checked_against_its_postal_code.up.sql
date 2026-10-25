-- The country a company's or contact's postal code and city resolve to, and
-- when that was asked. The read flags a stored country that differs; nothing
-- changes the stored country. NULL checked_at means not yet asked. A NULL
-- resolved country with a checked_at means the provider found no such place.
--
-- The trigger clears both whenever the postal code, city or country change, so
-- no writer has to remember to and the flag never speaks for an old address.
-- Derived from a public geocoder's answer, so not audited, like the geocode
-- point on company.

SET LOCAL lock_timeout = '3s';

ALTER TABLE company
    ADD COLUMN address_country_resolved text,
    ADD COLUMN address_country_checked_at timestamptz;

ALTER TABLE contact
    ADD COLUMN address_country_resolved text,
    ADD COLUMN address_country_checked_at timestamptz;

-- NULL for a place cached before the country was asked for.
ALTER TABLE geocode_cache ADD COLUMN country_code text;

CREATE FUNCTION address_country_check_goes_stale() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  NEW.address_country_resolved := NULL;
  NEW.address_country_checked_at := NULL;
  RETURN NEW;
END;
$$;

CREATE TRIGGER trg_company_address_country_check_stale
    BEFORE UPDATE OF address_postal_code, address_city, address_country ON company
    FOR EACH ROW
    WHEN (OLD.address_postal_code IS DISTINCT FROM NEW.address_postal_code
       OR OLD.address_city IS DISTINCT FROM NEW.address_city
       OR OLD.address_country IS DISTINCT FROM NEW.address_country)
    EXECUTE FUNCTION address_country_check_goes_stale();

CREATE TRIGGER trg_contact_address_country_check_stale
    BEFORE UPDATE OF address_postal_code, address_city, address_country ON contact
    FOR EACH ROW
    WHEN (OLD.address_postal_code IS DISTINCT FROM NEW.address_postal_code
       OR OLD.address_city IS DISTINCT FROM NEW.address_city
       OR OLD.address_country IS DISTINCT FROM NEW.address_country)
    EXECUTE FUNCTION address_country_check_goes_stale();
