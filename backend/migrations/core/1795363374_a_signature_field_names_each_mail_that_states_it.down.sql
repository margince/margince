SET LOCAL lock_timeout = '3s';

ALTER TABLE contact_profile_field DROP COLUMN IF EXISTS signature_support;
