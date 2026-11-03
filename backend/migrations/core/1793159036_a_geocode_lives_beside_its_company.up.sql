SET LOCAL lock_timeout = '3s';
-- The geocode result lives beside the company, in the row that already holds
-- its attempt ledger. Written on the company row, every lookup moved the
-- company's version, and an edit form opened before the answer refused to save.
ALTER TABLE company_geocode_state
    ADD COLUMN IF NOT EXISTS lat double precision,
    ADD COLUMN IF NOT EXISTS lon double precision,
    ADD COLUMN IF NOT EXISTS status text,
    ADD COLUMN IF NOT EXISTS provider text,
    ADD COLUMN IF NOT EXISTS input_hash text,
    ADD COLUMN IF NOT EXISTS geocoded_at timestamptz;
ALTER TABLE company_geocode_state ADD CONSTRAINT company_geocode_state_status_check
    CHECK (status IS NULL OR status IN ('ok', 'failed', 'no_match', 'stale')) NOT VALID;

-- An address change marks the point stale in the same transaction, now on the
-- side row. A company never resolved keeps no status, so the coordinates read
-- as absent rather than out of date.
CREATE OR REPLACE FUNCTION company_geocode_state_goes_stale() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  UPDATE company_geocode_state SET status = 'stale'
   WHERE company_id = NEW.id AND status IS NOT NULL AND status <> 'stale';
  RETURN NULL;
END;
$$;
CREATE TRIGGER trg_company_geocode_state_stale
    AFTER UPDATE OF address_line1, address_line2, address_city, address_region, address_postal_code, address_country
    ON company FOR EACH ROW
    WHEN (OLD.address_line1 IS DISTINCT FROM NEW.address_line1
       OR OLD.address_line2 IS DISTINCT FROM NEW.address_line2
       OR OLD.address_city IS DISTINCT FROM NEW.address_city
       OR OLD.address_region IS DISTINCT FROM NEW.address_region
       OR OLD.address_postal_code IS DISTINCT FROM NEW.address_postal_code
       OR OLD.address_country IS DISTINCT FROM NEW.address_country)
    EXECUTE FUNCTION company_geocode_state_goes_stale();

-- The company row's own stale mark has no reader left.
DROP TRIGGER IF EXISTS trg_company_geocode_stale ON company;
DROP FUNCTION IF EXISTS company_geocode_goes_stale();
