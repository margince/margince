-- The widened evidence source checks added beside this file are validated here,
-- in their own transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '5s';
ALTER TABLE company_fact VALIDATE CONSTRAINT company_fact_source_check;
ALTER TABLE company_profile_field VALIDATE CONSTRAINT company_profile_field_source_check;
