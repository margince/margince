SET LOCAL lock_timeout = '3s';
ALTER TABLE app_user DROP COLUMN IF EXISTS email_domain_folded;
ALTER TABLE company_domain DROP COLUMN IF EXISTS domain_folded;
ALTER TABLE activity_participant DROP COLUMN IF EXISTS address_domain_folded;
