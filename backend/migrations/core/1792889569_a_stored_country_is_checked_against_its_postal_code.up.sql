-- The countries a company's or contact's postal code and city resolve to, and
-- when that was asked. The read flags a stored country that is none of them;
-- nothing changes the stored country. No row means not yet asked. An empty set
-- means the provider found no such place.
--
-- One row per record in a table of its own, so writing the check never moves
-- the record's version and an open edit form keeps its If-Match. A trigger
-- deletes the row whenever the postal code, city or country change, so no
-- writer has to remember to and the flag never speaks for an old address.
-- Derived from a public geocoder's answer, so not audited, like the geocode
-- point on company.

SET LOCAL lock_timeout = '3s';

CREATE TABLE company_address_country (
    company_id uuid PRIMARY KEY REFERENCES company(id) ON DELETE CASCADE,
    countries text[] NOT NULL,
    checked_at timestamptz NOT NULL,
    CONSTRAINT company_address_country_codes_are_alpha2
        CHECK (array_to_string(countries, ',') ~ '^([A-Z]{2}(,[A-Z]{2})*)?$')
);

CREATE TABLE contact_address_country (
    contact_id uuid PRIMARY KEY REFERENCES contact(id) ON DELETE CASCADE,
    countries text[] NOT NULL,
    checked_at timestamptz NOT NULL,
    CONSTRAINT contact_address_country_codes_are_alpha2
        CHECK (array_to_string(countries, ',') ~ '^([A-Z]{2}(,[A-Z]{2})*)?$')
);

-- What a place query's candidates said about their countries, kept apart from
-- geocode_cache so a row never pairs one answer's point with another's country.
-- Like geocode_cache it holds a public place name and no customer data.
CREATE TABLE geocode_country_cache (
    query text PRIMARY KEY,
    country_codes text[] NOT NULL,
    provider text NOT NULL,
    resolved_at timestamptz NOT NULL,
    CONSTRAINT geocode_country_cache_codes_are_alpha2
        CHECK (array_to_string(country_codes, ',') ~ '^([A-Z]{2}(,[A-Z]{2})*)?$')
);

CREATE FUNCTION company_address_country_goes_stale() RETURNS trigger
    LANGUAGE plpgsql
    AS $$BEGIN
  DELETE FROM company_address_country WHERE company_id = NEW.id;
  RETURN NULL;
END;
$$;

CREATE FUNCTION contact_address_country_goes_stale() RETURNS trigger
    LANGUAGE plpgsql
    AS $$BEGIN
  DELETE FROM contact_address_country WHERE contact_id = NEW.id;
  RETURN NULL;
END;
$$;

CREATE TRIGGER trg_company_address_country_stale
    AFTER UPDATE OF address_postal_code, address_city, address_country ON company
    FOR EACH ROW
    WHEN (OLD.address_postal_code IS DISTINCT FROM NEW.address_postal_code
       OR OLD.address_city IS DISTINCT FROM NEW.address_city
       OR OLD.address_country IS DISTINCT FROM NEW.address_country)
    EXECUTE FUNCTION company_address_country_goes_stale();

CREATE TRIGGER trg_contact_address_country_stale
    AFTER UPDATE OF address_postal_code, address_city, address_country ON contact
    FOR EACH ROW
    WHEN (OLD.address_postal_code IS DISTINCT FROM NEW.address_postal_code
       OR OLD.address_city IS DISTINCT FROM NEW.address_city
       OR OLD.address_country IS DISTINCT FROM NEW.address_country)
    EXECUTE FUNCTION contact_address_country_goes_stale();
