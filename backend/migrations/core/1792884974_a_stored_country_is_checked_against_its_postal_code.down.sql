SET LOCAL lock_timeout = '3s';

DROP TRIGGER IF EXISTS trg_contact_address_country_check_stale ON contact;
DROP TRIGGER IF EXISTS trg_company_address_country_check_stale ON company;
DROP FUNCTION IF EXISTS address_country_check_goes_stale();

ALTER TABLE geocode_cache DROP COLUMN IF EXISTS country_code;

ALTER TABLE contact
    DROP COLUMN IF EXISTS address_country_checked_at,
    DROP COLUMN IF EXISTS address_country_resolved;

ALTER TABLE company
    DROP COLUMN IF EXISTS address_country_checked_at,
    DROP COLUMN IF EXISTS address_country_resolved;
