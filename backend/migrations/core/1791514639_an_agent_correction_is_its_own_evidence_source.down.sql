-- Narrowing back refuses any row an agent wrote, which is the honest failure:
-- the rows exist and this constraint would be a lie about them. NOT VALID is
-- what lets the narrowing land at all, since validating it would scan rows the
-- vocabulary no longer admits.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company_fact DROP CONSTRAINT company_fact_source_check;
ALTER TABLE company_fact ADD CONSTRAINT company_fact_source_check
  CHECK (source IN ('human', 'site_read', 'connector', 'migration', 'technical_lookup')) NOT VALID;

ALTER TABLE company_profile_field DROP CONSTRAINT company_profile_field_source_check;
ALTER TABLE company_profile_field ADD CONSTRAINT company_profile_field_source_check
  CHECK (source IN ('human', 'site_read', 'connector', 'migration')) NOT VALID;
