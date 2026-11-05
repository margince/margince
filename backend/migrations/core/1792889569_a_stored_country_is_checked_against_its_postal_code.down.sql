SET LOCAL lock_timeout = '3s';

DROP TRIGGER IF EXISTS trg_contact_address_country_stale ON contact;
DROP TRIGGER IF EXISTS trg_company_address_country_stale ON company;
DROP FUNCTION IF EXISTS contact_address_country_goes_stale();
DROP FUNCTION IF EXISTS company_address_country_goes_stale();

DROP TABLE IF EXISTS geocode_country_cache;
DROP TABLE IF EXISTS contact_address_country;
DROP TABLE IF EXISTS company_address_country;
