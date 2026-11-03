SET LOCAL lock_timeout = '3s';
ALTER TABLE lead DROP COLUMN IF EXISTS email_domain_folded;
ALTER TABLE contact_email DROP COLUMN IF EXISTS email_domain_folded;
